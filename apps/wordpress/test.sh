#!/usr/bin/env bash
# Smoke test for a built WordPress image. Usage: test.sh <image-ref>
# Boots the container (php-fpm + nginx via supervisord on :8080), then asserts the
# WordPress stack is healthy WITHOUT a database: with no wp-config.php present, the
# WordPress front controller 302-redirects to the setup wizard, and the setup page
# itself serves HTTP 200 with the WordPress branding. A read-only rootfs is fine;
# nginx/php only need a writable /tmp for pid/sockets/temp/session scratch.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="wordpress-smoke-$$"
PORT=8099

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

# must be the nonroot wordpress user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

# the shipped WordPress version file must be present (proves the core tree is there)
docker run --rm --entrypoint /usr/bin/php "$IMAGE" \
  -r 'require "/var/www/html/wp-includes/version.php"; echo "wp_version=".$wp_version."\n";' \
  | grep -q 'wp_version=' || { echo "wp-includes/version.php missing or unreadable"; exit 1; }

docker run --rm -d --name "$NAME" \
  --read-only --tmpfs /tmp \
  -p "127.0.0.1:${PORT}:8080" \
  "$IMAGE" >/dev/null

# wait for nginx to accept connections and the PHP front controller to answer
ok=""
for i in $(seq 1 30); do
  # no config yet -> WordPress issues a 302 to /wp-admin/setup-config.php
  code="$(curl -fsS -o /dev/null -w '%{http_code}' "http://127.0.0.1:${PORT}/" 2>/dev/null || true)"
  case "$code" in 200|301|302) ok=1; break;; esac
  sleep 1
done
[ -n "$ok" ] || { echo "WordPress did not answer over HTTP"; docker logs "$NAME" 2>&1 | tail -30; exit 1; }
echo "front controller responded (HTTP $code)"

# the setup wizard must serve HTTP 200 and look like WordPress
setup_code="$(curl -fsSL -o /tmp/wp-setup.html -w '%{http_code}' "http://127.0.0.1:${PORT}/wp-admin/setup-config.php" 2>/dev/null || true)"
[ "$setup_code" = "200" ] || { echo "setup-config.php did not serve HTTP 200 (got $setup_code)"; docker logs "$NAME" 2>&1 | tail -30; exit 1; }
grep -qi 'wordpress' /tmp/wp-setup.html || { echo "setup page does not mention WordPress"; exit 1; }

echo "smoke test passed (php-fpm+nginx up, setup wizard serves HTTP 200, nonroot user: $user)"
