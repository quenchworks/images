#!/usr/bin/env bash
# Smoke test for a built coolify-app image. Usage: test.sh <image-ref>
#
# coolify-app is the PHP 8.4 / Laravel 12 app: php-fpm (the app on :9000) + nginx (:8080)
# + the ported s6 services (bootstrap one-shots + scheduler/horizon/nightwatch workers),
# all under supervisord. A full boot needs Postgres + Redis (the chart provides them);
# this smoke test runs WITHOUT them and proves the pieces that don't need a live DB:
#   * the image boots under a READ-ONLY rootfs with the exact writable carve-outs the
#     chart ships (tmpfs /tmp + emptyDir-style storage/ + bootstrap/cache + .env)
#   * php-fpm comes up and answers FastCGI -> nginx serves the static /healthcheck 200
#   * the Laravel app boots: `php artisan --version` prints a version (autoloader +
#     config load OK), and the required PHP extensions (pdo_pgsql, redis) are loaded
#   * the Vite build assets are present (public/build/manifest.json)
#   * runtime tooling is present (ssh, git, psql) and the user is nonroot uid 9999
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-coolify-app-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE (read-only rootfs; tmpfs /tmp + writable storage + bootstrap/cache)"
# Disable the workers so the test doesn't hang on a missing DB/Redis; we only need
# php-fpm + nginx for the HTTP probes. SCHEDULER/HORIZON disabled => wrappers sleep; the
# bootstrap one-shot still runs but its DB probe times out gracefully (non-fatal). .env is
# a symlink to /tmp/coolify.env, so the writable /tmp tmpfs covers it -- no separate mount.
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /tmp:rw,mode=1777,uid=9999,gid=9999 \
  --tmpfs /var/www/html/storage:rw,uid=9999,gid=9999 \
  --tmpfs /var/www/html/bootstrap/cache:rw,uid=9999,gid=9999 \
  -e SCHEDULER_ENABLED=false \
  -e HORIZON_ENABLED=false \
  -e NIGHTWATCH_ENABLED=false \
  -p 127.0.0.1:8080:8080 \
  "$IMAGE" >/dev/null

wait_200() {
  local url="$1" label="$2" i code
  for i in $(seq 1 60); do
    code="$(curl -s -o /dev/null -w '%{http_code}' "$url" 2>/dev/null || true)"
    if [ "$code" = "200" ]; then echo "$label OK after ${i}s"; return 0; fi
    if ! docker ps --format '{{.Names}}' | grep -q "^${NAME}$"; then
      echo "container exited early"; docker logs "$NAME"; exit 1
    fi
    sleep 1
  done
  echo "$label did not become healthy (got '$code')"; docker logs "$NAME"; exit 1
}

# nginx serves the static healthcheck, which FastCGI-passes to php-fpm -> a 200 here
# proves BOTH nginx (:8080) and php-fpm (:9000) are up and wired together.
wait_200 "http://127.0.0.1:8080/healthcheck" "nginx+php-fpm /healthcheck"
curl -fsS "http://127.0.0.1:8080/healthcheck" | grep -q "OK" \
  || { echo "/healthcheck payload unexpected"; docker logs "$NAME"; exit 1; }

# The Laravel app must boot to print its version (autoloader + config load OK).
echo "artisan version:"
docker exec -w /var/www/html "$NAME" php artisan --version \
  || { echo "php artisan --version failed (app did not boot)"; docker logs "$NAME"; exit 1; }

# Required PHP extensions must be loaded in the runtime.
for ext in pdo_pgsql pgsql redis bcmath gd intl mbstring pcntl; do
  docker exec "$NAME" php -m | grep -qi "^${ext}$" \
    || { echo "PHP extension '$ext' missing from runtime"; docker exec "$NAME" php -m; exit 1; }
done
echo "PHP extensions present: pdo_pgsql pgsql redis bcmath gd intl mbstring pcntl"

# Vite build assets must be present.
docker exec "$NAME" test -f /var/www/html/public/build/manifest.json \
  || { echo "Vite build assets (public/build/manifest.json) missing"; exit 1; }
echo "Vite build assets present"

# Runtime tooling coolify shells out to.
for bin in ssh git git-lfs psql php-fpm nginx supervisord; do
  docker exec "$NAME" sh -c "command -v $bin" >/dev/null \
    || { echo "runtime tool '$bin' missing"; exit 1; }
done
echo "runtime tooling present: ssh git git-lfs psql php-fpm nginx supervisord"

# must run as the nonroot www-data user (uid 9999)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "9999" ] || { echo "expected user 9999, got '$user'"; exit 1; }

echo "php version:"; docker exec "$NAME" php --version | head -1
echo "smoke test passed (nonroot uid $user, read-only rootfs; nginx :8080 + php-fpm :9000 healthy; artisan boots; extensions + assets + tooling present)"
