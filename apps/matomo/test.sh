#!/usr/bin/env bash
# Smoke test for a built matomo image. Usage: test.sh <image-ref>
#
# Matomo is the PHP web-analytics app: php-fpm (:9000) + nginx (:8080) under
# supervisord. A full install needs MySQL/MariaDB (the chart provides it); this smoke
# test runs WITHOUT a DB and proves the pieces that don't need one:
#   * the image boots under a READ-ONLY rootfs with the exact writable carve-outs the
#     chart ships (tmpfs /tmp + emptyDir-style /var/www/html/tmp + /var/www/html/config)
#   * php-fpm comes up and answers FastCGI -> nginx serves Matomo's installer at "/"
#     with HTTP 200 (autoloader + core boot OK; no DB configured yet)
#   * the installer page really is Matomo (contains "Matomo")
#   * the Matomo CLI boots and prints the expected version
#   * the required PHP extensions are loaded, and the user is nonroot uid 1001
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-matomo-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE (read-only rootfs; tmpfs /tmp + writable tmp/ + config/)"
# Mount tmpfs over /tmp (supervisord/nginx/php scratch), Matomo's writable cache tree
# /var/www/html/tmp, and /var/www/html/config. Matomo REQUIRES config/ to be writable at
# boot (it fatals with a 500 otherwise). The config tmpfs starts empty, so the entrypoint
# seeds it from the baked config-default/ (global.ini.php ...) -- exactly the seed-if-empty
# flow the chart's config PVC relies on.
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /tmp:rw,mode=1777,uid=1001,gid=1001 \
  --tmpfs /var/www/html/tmp:rw,uid=1001,gid=1001 \
  --tmpfs /var/www/html/config:rw,uid=1001,gid=1001 \
  -p 127.0.0.1:8080:8080 \
  "$IMAGE" >/dev/null

wait_200() {
  local url="$1" label="$2" i code
  for i in $(seq 1 60); do
    code="$(curl -s -o /dev/null -w '%{http_code}' "$url" 2>/dev/null || true)"
    if [ "$code" = "200" ]; then echo "$label OK after ${i}s (HTTP 200)"; return 0; fi
    if ! docker ps --format '{{.Names}}' | grep -q "^${NAME}$"; then
      echo "container exited early"; docker logs "$NAME"; exit 1
    fi
    sleep 1
  done
  echo "$label did not become healthy (got '$code')"; docker logs "$NAME"; exit 1
}

# nginx (:8080) FastCGI-passes / -> index.php -> php-fpm (:9000). A 200 here proves
# BOTH nginx and php-fpm are up and wired, and Matomo boots into its installer.
wait_200 "http://127.0.0.1:8080/index.php" "matomo installer (nginx+php-fpm)"

body="$(curl -fsS "http://127.0.0.1:8080/index.php")"
# grep without -q consumes all input (a -q here would close the pipe early and, under
# `set -o pipefail`, SIGPIPE curl/printf into a false failure).
printf '%s' "$body" | grep -i "matomo" >/dev/null \
  || { echo "installer page does not look like Matomo"; printf '%s' "$body" | head -40; docker logs "$NAME"; exit 1; }
echo "installer page identifies as Matomo"

# The Matomo CLI must boot to print its version (autoloader + core load OK).
echo "matomo console version:"
docker exec -w /var/www/html "$NAME" php console --version \
  || { echo "php console --version failed (app did not boot)"; docker logs "$NAME"; exit 1; }

# Required PHP extensions must be loaded in the runtime.
mods="$(docker exec "$NAME" php -m)"
for ext in pdo_mysql mysqli gd mbstring dom curl openssl zip gmp intl; do
  printf '%s' "$mods" | grep -i "^${ext}$" >/dev/null \
    || { echo "PHP extension '$ext' missing from runtime"; printf '%s' "$mods"; exit 1; }
done
# opcache reports as "Zend OPcache" in php -m, not "opcache".
printf '%s' "$mods" | grep -i "opcache" >/dev/null \
  || { echo "PHP extension 'opcache' missing from runtime"; printf '%s' "$mods"; exit 1; }
echo "PHP extensions present: pdo_mysql mysqli gd mbstring dom curl openssl zip gmp intl opcache"

# must run as the nonroot matomo user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "php version:"; docker exec "$NAME" php --version | head -1
echo "smoke test passed (nonroot uid $user, read-only rootfs; nginx :8080 + php-fpm :9000 healthy; Matomo installer serves 200; CLI boots; extensions present)"
