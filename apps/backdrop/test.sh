#!/usr/bin/env bash
# Smoke test for a built Backdrop CMS image. Usage: test.sh <image-ref> [version]
# Boots php-fpm + nginx on a read-only rootfs with files/ on a tmpfs and the database
# from BACKDROP_DB_* env, requires the web installer to answer, runs the real CLI
# installer against a throwaway MariaDB, then requires the front page to render the site
# name and the login form to render. Proves the PHP extensions, the MySQL driver, the
# env-driven read-only settings.php and the nginx clean-URL routing, not just a port.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="backdrop-smoke-$$"
DB="backdrop-smoke-db-$$"
NET="backdrop-smoke-net-$$"
PORT=8096
BASE="http://127.0.0.1:${PORT}"

cleanup() { docker rm -f "$NAME" "$DB" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
trap cleanup EXIT
fail() { echo "$1"; docker logs "$NAME" 2>&1 | tail -40; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

docker network create "$NET" >/dev/null
docker run -d --name "$DB" --network "$NET" \
  -e MARIADB_ROOT_PASSWORD=rootpw -e MARIADB_DATABASE=backdrop -e MARIADB_USER=backdrop -e MARIADB_PASSWORD=backdroppw \
  mariadb:11 >/dev/null
for _ in $(seq 1 60); do
  docker exec "$DB" mariadb -ubackdrop -pbackdroppw -e 'select 1' backdrop >/dev/null 2>&1 && break
  sleep 1
done
docker exec "$DB" mariadb -ubackdrop -pbackdroppw -e 'select 1' backdrop >/dev/null 2>&1 || { docker logs "$DB" | tail -20; echo "mariadb never came up"; exit 1; }

docker run -d --name "$NAME" --network "$NET" \
  --read-only --tmpfs /tmp \
  --tmpfs /var/www/html/files:mode=1777 \
  -e BACKDROP_DB_HOST="$DB" -e BACKDROP_DB_NAME=backdrop -e BACKDROP_DB_USER=backdrop \
  -e BACKDROP_DB_PASSWORD=backdroppw -e BACKDROP_HASH_SALT=smoke-salt-0123456789 \
  -p "127.0.0.1:${PORT}:8080" \
  "$IMAGE" >/dev/null

for _ in $(seq 1 30); do
  code="$(curl -sS -o /dev/null -w '%{http_code}' "$BASE/core/install.php" 2>/dev/null || true)"
  [ "$code" = "200" ] && break
  sleep 1
done
[ "${code:-}" = "200" ] || fail "the web installer did not answer HTTP 200 (got ${code:-none})"
echo "web installer answered before install"

docker exec "$NAME" php core/scripts/install.sh \
  --db-url="mysql://backdrop:backdroppw@${DB}/backdrop" \
  --account-name=smokeadmin --account-pass=Quench-Smoke-Pass-2026 --account-mail=smoke@example.com \
  --site-name=QuenchSmoke --site-mail=smoke@example.com > /tmp/backdrop-install.log 2>&1 \
  || { cat /tmp/backdrop-install.log; fail "the CLI installer failed"; }
cat /tmp/backdrop-install.log
grep -q 'installed successfully' /tmp/backdrop-install.log || fail "the CLI installer did not report success"
docker exec "$NAME" test -s /var/www/html/files/config/active/system.core.json || fail "no active config on the files volume"

# CLI scripts, config and backups are never served.
for p in core/scripts/install.sh files/config/active/system.core.json core/includes/bootstrap.inc; do
  c="$(curl -sS -o /dev/null -w '%{http_code}' "$BASE/$p" || true)"
  [ "$c" = "403" ] || fail "/$p is reachable over HTTP (got $c)"
done

page="$(curl -sS -w '\n%{http_code}' "$BASE/" || true)"
[ "$(tail -n1 <<<"$page")" = "200" ] || { head -30 <<<"$page"; fail "front page did not serve HTTP 200"; }
grep -q 'QuenchSmoke' <<<"$page" || { head -40 <<<"$page"; fail "front page did not render the site name"; }

login="$(curl -sS -w '\n%{http_code}' "$BASE/user/login" || true)"
[ "$(tail -n1 <<<"$login")" = "200" ] || fail "/user/login (clean URL) did not serve HTTP 200"
grep -q 'name="pass"' <<<"$login" || fail "/user/login did not render the login form"

ver="$(docker exec "$NAME" php -r 'define("BACKDROP_ROOT", getcwd()); $s = file_get_contents("core/includes/bootstrap.inc"); preg_match("/BACKDROP_VERSION., .([0-9.]+)/", $s, $m); echo $m[1];')"
echo "backdrop: $ver"
[ -z "$WANT" ] || [ "$ver" = "$WANT" ] || fail "expected Backdrop $WANT, the tree says $ver"

mods="$(docker exec "$NAME" php -m)"
for ext in pdo_mysql gd mbstring curl zip intl apcu dom zlib; do
  grep -qix "$ext" <<<"$mods" || fail "PHP extension $ext missing"
done

if docker logs "$NAME" 2>&1 | grep -E 'PHP (Fatal|Parse) error|Uncaught'; then fail "php logged a fatal error"; fi
echo "smoke test passed (backdrop ${ver}, uid $user, installed, front page + login served)"
