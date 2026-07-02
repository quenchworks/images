#!/usr/bin/env bash
# Smoke test for a built Nextcloud image. Usage: test.sh <image-ref>
#
# Nextcloud is the PHP app under php-fpm (:9000) + nginx (:8080), supervised by
# supervisord. A full install needs a database (MySQL/MariaDB/PostgreSQL) + an
# admin account -- the chart wires those. This smoke test runs WITHOUT a DB and
# proves the pieces that don't need one:
#   * the image boots under a READ-ONLY rootfs with the exact writable carve-outs
#     the chart ships (tmpfs /tmp + data/ + config/ + custom_apps/)
#   * php-fpm comes up and answers FastCGI -> nginx serves /status.php as 200 JSON
#     that reports the Nextcloud version (installed:false pre-install)
#   * the index/front controller serves (200/redirect to the setup wizard)
#   * occ works: `php occ --version` prints the version (no DB needed)
#   * the required + recommended PHP extensions are loaded
#   * the user is nonroot uid 1001
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-nextcloud-smoke-$$"
PORT=8087

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

# must run as the nonroot nextcloud user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "starting $IMAGE (read-only rootfs; tmpfs /tmp + data + config + custom_apps)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /tmp:rw,mode=1777,uid=1001,gid=1001 \
  --tmpfs /var/www/html/data:rw,uid=1001,gid=1001 \
  --tmpfs /var/www/html/config:rw,uid=1001,gid=1001 \
  --tmpfs /var/www/html/custom_apps:rw,uid=1001,gid=1001 \
  -p "127.0.0.1:${PORT}:8080" \
  "$IMAGE" >/dev/null

wait_200() {
  local url="$1" label="$2" i code
  for i in $(seq 1 60); do
    code="$(curl -s -o /tmp/nc-out -w '%{http_code}' "$url" 2>/dev/null || true)"
    if [ "$code" = "200" ]; then echo "$label OK after ${i}s"; return 0; fi
    if ! docker ps --format '{{.Names}}' | grep -q "^${NAME}$"; then
      echo "container exited early"; docker logs "$NAME"; exit 1
    fi
    sleep 1
  done
  echo "$label did not become healthy (got '$code')"; docker logs "$NAME"; exit 1
}

# nginx serves /status.php by FastCGI-passing to php-fpm -> a 200 here proves BOTH
# nginx (:8080) and php-fpm (:9000) are up and wired together.
wait_200 "http://127.0.0.1:${PORT}/status.php" "nginx+php-fpm /status.php"
cat /tmp/nc-out; echo
grep -qi 'nextcloud' /tmp/nc-out || { echo "/status.php did not mention Nextcloud"; docker logs "$NAME"; exit 1; }
grep -qi '"version"' /tmp/nc-out || { echo "/status.php has no version field"; exit 1; }
echo "/status.php reports the Nextcloud version"

# The front controller must render the Nextcloud app. Pre-install (no DB) Nextcloud
# renders its setup/login page but replies HTTP 503 ("not installed / not ready"),
# so accept the pre-install statuses and assert the body is really Nextcloud.
code="$(curl -s -o /tmp/nc-index -w '%{http_code}' "http://127.0.0.1:${PORT}/" 2>/dev/null || true)"
case "$code" in
  200|302|303|503) echo "front controller serves (HTTP $code)";;
  *) echo "front controller unexpected HTTP '$code'"; docker logs "$NAME"; exit 1;;
esac
grep -qi 'nextcloud' /tmp/nc-index \
  || { echo "front controller body is not the Nextcloud app"; docker logs "$NAME"; exit 1; }
echo "front controller rendered the Nextcloud app"

# occ must run and print the Nextcloud version (no DB needed).
echo "occ version:"
docker exec -w /var/www/html "$NAME" php occ --version --no-warnings \
  || { echo "php occ --version failed"; docker logs "$NAME"; exit 1; }

# Required + recommended PHP extensions must be present in the runtime.
for ext in ctype curl dom gd fileinfo mbstring openssl posix simplexml \
           xmlreader xmlwriter zip pdo pdo_mysql pdo_pgsql pdo_sqlite \
           intl bcmath gmp exif sodium apcu redis imagick ldap; do
  docker exec "$NAME" php -m | grep -qi "^${ext}$" \
    || { echo "PHP extension '$ext' missing"; docker exec "$NAME" php -m; exit 1; }
done
# opcache is reported as "Zend OPcache" by php -m.
docker exec "$NAME" php -m | grep -qi "opcache" \
  || { echo "PHP extension 'opcache' missing"; docker exec "$NAME" php -m; exit 1; }
echo "PHP extensions present (required + recommended)"

# runtime pieces
for bin in php php-fpm nginx supervisord; do
  docker exec "$NAME" sh -c "command -v $bin" >/dev/null \
    || { echo "runtime tool '$bin' missing"; exit 1; }
done

echo "php version:"; docker exec "$NAME" php --version | head -1
echo "smoke test passed (nonroot uid $user, read-only rootfs; nginx :8080 + php-fpm :9000 healthy; status.php + occ + extensions OK)"
