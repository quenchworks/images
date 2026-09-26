#!/usr/bin/env bash
# Smoke test for a built MediaWiki image. Usage: test.sh <image-ref> [version]
# Boots php-fpm + nginx on a read-only rootfs, installs a real wiki into SQLite on
# /tmp with maintenance/run.php install, then requires the Main Page to render and
# the API to report the expected version. This proves the PHP extensions, the
# vendor tree, the database driver and the nginx routing, not just a port.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="mediawiki-smoke-$$"
PORT=8099
BASE="http://127.0.0.1:${PORT}"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT
fail() { echo "$1"; docker logs "$NAME" 2>&1 | tail -40; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

docker run -d --name "$NAME" \
  --read-only --tmpfs /tmp --tmpfs /var/www/html/images:mode=1777 \
  -e MW_CONFIG_FILE=/tmp/mw/LocalSettings.php \
  -p "127.0.0.1:${PORT}:8080" \
  "$IMAGE" >/dev/null

for i in $(seq 1 30); do
  code="$(curl -sS -o /dev/null -w '%{http_code}' "$BASE/index.php" 2>/dev/null || true)"
  case "$code" in 200|301|302|500) break;; esac
  sleep 1
done
[ -n "${code:-}" ] && [ "$code" != "000" ] || fail "nginx never answered"
echo "front controller answered before install (HTTP $code)"

docker exec "$NAME" sh -c 'mkdir -p /tmp/mw /tmp/mwdata && cd /var/www/html && \
  php maintenance/run.php install --confpath=/tmp/mw --dbtype=sqlite --dbpath=/tmp/mwdata \
    --server=http://127.0.0.1:8099 --scriptpath="" --pass=Quench-Smoke-Pass-2026 \
    QuenchSmoke admin' > /tmp/mw-install.log 2>&1 \
  || { cat /tmp/mw-install.log; fail "maintenance/run.php install failed"; }
docker exec "$NAME" test -f /tmp/mw/LocalSettings.php || fail "installer wrote no LocalSettings.php"
echo "installed into SQLite"

page="$(curl -sS -w '\n%{http_code}' "$BASE/index.php?title=Main_Page" || true)"
[ "$(tail -n1 <<<"$page")" = "200" ] || fail "Main_Page did not serve HTTP 200"
grep -q 'MediaWiki has been installed' <<<"$page" || { head -40 <<<"$page"; fail "Main_Page did not render the default page"; }

gen="$(curl -fsS "$BASE/api.php?action=query&meta=siteinfo&format=json" | sed -n 's/.*"generator":"\([^"]*\)".*/\1/p')"
echo "api generator: $gen"
case "$gen" in "MediaWiki "*) ;; *) fail "api.php did not report a MediaWiki generator";; esac
[ -z "$WANT" ] || [ "$gen" = "MediaWiki $WANT" ] || fail "expected MediaWiki $WANT"

mcode="$(curl -sS -o /dev/null -w '%{http_code}' "$BASE/maintenance/run.php" || true)"
[ "$mcode" = "403" ] || fail "maintenance/ is reachable over HTTP (got $mcode)"

echo "smoke test passed ($gen, SQLite install, Main_Page 200, maintenance/ 403, nonroot user: $user)"
