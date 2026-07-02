#!/usr/bin/env bash
# Smoke test for a built Drupal image. Usage: test.sh <image-ref>
#
# Drupal is a PHP-FPM app fronted by nginx (both under supervisord). A real site needs
# a database (the chart wires MySQL/MariaDB or PostgreSQL); this smoke test runs WITHOUT
# one and proves the pieces that don't need a live DB:
#   * the image boots under a READ-ONLY rootfs with the writable carve-outs the chart
#     ships (tmpfs /tmp + emptyDir-style sites/default/files)
#   * php-fpm comes up and answers FastCGI -> nginx serves HTTP; with no settings.php
#     Drupal redirects "/" to the installer and /core/install.php returns HTTP 200
#   * the served page is the Drupal installer (mentions Drupal / the install profile)
#   * php reports a version, the required extensions are loaded, and the Drupal core
#     version constant is present in the shipped tree
#   * the container runs as the nonroot drupal user (uid 1001)
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-drupal-smoke-$$"
PORT=8098

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

# must be the nonroot drupal user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "starting $IMAGE (read-only rootfs; tmpfs /tmp + writable sites/default/files)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /tmp:rw,mode=1777,uid=1001,gid=1001 \
  --tmpfs /opt/drupal/sites/default/files:rw,uid=1001,gid=1001 \
  -p "127.0.0.1:${PORT}:8080" \
  "$IMAGE" >/dev/null

wait_code() {
  local url="$1" want="$2" label="$3" i code
  for i in $(seq 1 60); do
    code="$(curl -s -o /tmp/drupal-out.html -w '%{http_code}' "$url" 2>/dev/null || true)"
    if [[ "$code" =~ ^($want)$ ]]; then echo "$label -> HTTP $code (after ${i}s)"; return 0; fi
    if ! docker ps --format '{{.Names}}' | grep -q "^${NAME}$"; then
      echo "container exited early"; docker logs "$NAME"; exit 1
    fi
    sleep 1
  done
  echo "$label did not return $want (last '$code')"; docker logs "$NAME" 2>&1 | tail -30; exit 1
}

# "/" with no settings.php redirects (302) to the installer; /core/install.php is a 200.
wait_code "http://127.0.0.1:${PORT}/" "200|301|302" "GET / (nginx+php-fpm)"
wait_code "http://127.0.0.1:${PORT}/core/install.php" "200" "GET /core/install.php"

# The served page must be the Drupal installer.
grep -qi 'drupal' /tmp/drupal-out.html || {
  echo "install page does not mention Drupal"; head -40 /tmp/drupal-out.html; exit 1; }

# php version + required extensions loaded in the runtime.
echo "php version:"; docker exec "$NAME" php --version | head -1
for ext in gd pdo pdo_mysql pdo_pgsql pdo_sqlite mbstring dom simplexml xml ctype curl openssl fileinfo; do
  docker exec "$NAME" php -m | grep -qi "^${ext}$" \
    || { echo "PHP extension '$ext' missing"; docker exec "$NAME" php -m; exit 1; }
done
# OPcache registers as a Zend extension ("Zend OPcache"), not a plain module line.
docker exec "$NAME" php -m | grep -qi opcache \
  || { echo "OPcache not loaded"; docker exec "$NAME" php -m; exit 1; }
echo "PHP extensions present: gd pdo pdo_mysql pdo_pgsql pdo_sqlite mbstring dom simplexml xml ctype curl openssl fileinfo opcache(Zend)"

# Drupal core version present in the shipped tree.
echo -n "drupal core version: "
docker exec "$NAME" sh -c "grep -m1 \"const VERSION\" /opt/drupal/core/lib/Drupal.php" \
  || { echo "core version constant missing"; exit 1; }

# php-fpm + nginx + supervisord binaries present.
for bin in php-fpm nginx supervisord; do
  docker exec "$NAME" sh -c "command -v $bin" >/dev/null \
    || { echo "runtime tool '$bin' missing"; exit 1; }
done

echo "smoke test passed (nonroot uid $user, read-only rootfs; nginx :8080 + php-fpm :9000 serve the Drupal installer)"
