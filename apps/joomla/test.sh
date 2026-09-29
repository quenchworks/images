#!/usr/bin/env bash
# Smoke test for a built Joomla image. Usage: test.sh <image-ref> [version]
# Boots php-fpm + nginx on a read-only rootfs with the config dir on a volume, runs the
# real CLI installer against a throwaway MariaDB, then requires the front page and the
# administrator login to render and the CLI to report the expected version. This proves
# the PHP extensions, the database driver, the JPATH_CONFIGURATION wiring and the nginx
# routing, not just a port.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="joomla-smoke-$$"
DB="joomla-smoke-db-$$"
NET="joomla-smoke-net-$$"
PORT=8097
BASE="http://127.0.0.1:${PORT}"

cleanup() { docker rm -f "$NAME" "$DB" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
trap cleanup EXIT
fail() { echo "$1"; docker logs "$NAME" 2>&1 | tail -40; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

docker network create "$NET" >/dev/null
docker run -d --name "$DB" --network "$NET" \
  -e MARIADB_ROOT_PASSWORD=rootpw -e MARIADB_DATABASE=joomla -e MARIADB_USER=joomla -e MARIADB_PASSWORD=joomlapw \
  mariadb:11 >/dev/null
for _ in $(seq 1 60); do
  docker exec "$DB" mariadb -ujoomla -pjoomlapw -e 'select 1' joomla >/dev/null 2>&1 && break
  sleep 1
done
docker exec "$DB" mariadb -ujoomla -pjoomlapw -e 'select 1' joomla >/dev/null 2>&1 || { docker logs "$DB" | tail -20; echo "mariadb never came up"; exit 1; }

docker run -d --name "$NAME" --network "$NET" \
  --read-only --tmpfs /tmp \
  --tmpfs /var/www/html/images:mode=1777 --tmpfs /var/www/config:mode=1777 \
  -e JOOMLA_CONFIG_DIR=/var/www/config \
  -p "127.0.0.1:${PORT}:8080" \
  "$IMAGE" >/dev/null

for _ in $(seq 1 30); do
  code="$(curl -sS -o /dev/null -w '%{http_code}' "$BASE/" 2>/dev/null || true)"
  case "$code" in 200|301|302|500) break;; esac
  sleep 1
done
[ -n "${code:-}" ] && [ "$code" != "000" ] || fail "nginx never answered"
echo "front controller answered before install (HTTP $code)"

docker exec "$NAME" sh -c 'cd /var/www/html && php installation/joomla.php install \
  --site-name=QuenchSmoke --admin-user="Smoke Admin" --admin-username=smokeadmin \
  --admin-password="Quench-Smoke-Pass-2026" --admin-email=smoke@example.com \
  --db-type=mysqli --db-host='"$DB"' --db-user=joomla --db-pass=joomlapw --db-name=joomla \
  --db-prefix=qw_ --db-encryption=0 --no-interaction' > /tmp/joomla-install.log 2>&1 \
  || { cat /tmp/joomla-install.log; fail "the CLI installer failed"; }
docker exec "$NAME" test -s /var/www/config/configuration.php || fail "installer wrote no configuration.php on the config volume"
docker exec "$NAME" test ! -e /var/www/html/configuration.php || fail "configuration.php landed in the read-only docroot"
echo "installed into MariaDB, configuration.php on the config volume"

# The web installer stays out of reach once a site exists, and the CLI dir is never served.
for p in installation/index.php cli/joomla.php; do
  c="$(curl -sS -o /dev/null -w '%{http_code}' "$BASE/$p" || true)"
  [ "$c" = "403" ] || fail "/$p is reachable over HTTP (got $c)"
done

page="$(curl -sS -w '\n%{http_code}' "$BASE/" || true)"
[ "$(tail -n1 <<<"$page")" = "200" ] || { head -30 <<<"$page"; fail "front page did not serve HTTP 200"; }
grep -qi 'QuenchSmoke' <<<"$page" || { head -40 <<<"$page"; fail "front page did not render the site name"; }

adm="$(curl -sS -w '\n%{http_code}' "$BASE/administrator/" || true)"
[ "$(tail -n1 <<<"$adm")" = "200" ] || fail "administrator login did not serve HTTP 200"
grep -qi 'mod-login-username\|name="username"' <<<"$adm" || fail "administrator did not render the login form"

ver="$(docker exec "$NAME" sh -c 'cd /var/www/html && php cli/joomla.php --version' 2>&1 | head -3 | tr '\n' ' ')"
echo "cli: $ver"
[ -z "$WANT" ] || grep -qF "$WANT" <<<"$ver" || fail "expected Joomla $WANT, cli said: $ver"

if docker logs "$NAME" 2>&1 | grep -E 'PHP (Fatal|Parse) error|Uncaught'; then fail "php logged a fatal error"; fi
echo "smoke test passed (joomla ${WANT:-?}, uid $user, installed, front page + administrator served)"
