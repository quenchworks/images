#!/usr/bin/env bash
# Smoke test for a built Friendica image. Usage: test.sh <image-ref> [version]
# Boots php-fpm + nginx + the worker daemon on a read-only rootfs with the config dir on a
# volume, runs the real CLI installer (bin/console.php autoinstall) against a throwaway
# MariaDB, then requires nodeinfo to report the expected version, the front page to render
# the login form, the worker daemon to process queued jobs, and the code/config paths to stay hidden.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="friendica-smoke-$$"
DB="friendica-smoke-db-$$"
NET="friendica-smoke-net-$$"
PORT=8098
BASE="http://127.0.0.1:${PORT}"

cleanup() { docker rm -f "$NAME" "$DB" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
trap cleanup EXIT
fail() { echo "$1"; docker logs "$NAME" 2>&1 | tail -60; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

docker network create "$NET" >/dev/null
docker run -d --name "$DB" --network "$NET" \
  -e MARIADB_ROOT_PASSWORD=rootpw -e MARIADB_DATABASE=friendica -e MARIADB_USER=friendica -e MARIADB_PASSWORD=friendicapw \
  mariadb:11 >/dev/null
for _ in $(seq 1 60); do
  docker exec "$DB" mariadb -ufriendica -pfriendicapw -e 'select 1' friendica >/dev/null 2>&1 && break
  sleep 1
done
docker exec "$DB" mariadb -ufriendica -pfriendicapw -e 'select 1' friendica >/dev/null 2>&1 || { docker logs "$DB" | tail -20; echo "mariadb never came up"; exit 1; }

docker run -d --name "$NAME" --network "$NET" \
  --read-only --tmpfs /tmp --tmpfs /var/friendica/config:mode=1777 \
  -p "127.0.0.1:${PORT}:8080" \
  "$IMAGE" >/dev/null

for _ in $(seq 1 30); do
  code="$(curl -sS -o /dev/null -w '%{http_code}' "$BASE/" 2>/dev/null || true)"
  case "$code" in 200|301|302|500) break;; esac
  sleep 1
done
[ -n "${code:-}" ] && [ "$code" != "000" ] || fail "nginx never answered"
echo "front controller answered before install (HTTP $code)"

docker exec "$NAME" php bin/console.php autoinstall \
  --dbhost "$DB" --dbuser friendica --dbpass friendicapw --dbdata friendica \
  --admin admin@example.com --url "$BASE" > /tmp/friendica-install.log 2>&1 \
  || { cat /tmp/friendica-install.log; fail "autoinstall failed"; }
docker exec "$NAME" test -s /var/friendica/config/local.config.php || fail "installer wrote no local.config.php on the config volume"
echo "installed into MariaDB, local.config.php on the config volume"

ni="$(curl -sS -w '\n%{http_code}' "$BASE/nodeinfo/2.0" || true)"
[ "$(tail -n1 <<<"$ni")" = "200" ] || { head -20 <<<"$ni"; fail "/nodeinfo/2.0 did not serve HTTP 200"; }
sw="$(head -n -1 <<<"$ni" | jq -r '.software.name + " " + .software.version')"
echo "nodeinfo: $sw"
grep -q '^friendica ' <<<"$sw" || fail "nodeinfo did not name friendica"
[ -z "$WANT" ] || grep -qF " $WANT" <<<"$sw" || fail "expected Friendica $WANT, nodeinfo said: $sw"

# The first requests after the install run the post-install database update and answer
# with a maintenance page until it finishes, so poll.
ok=""
for _ in $(seq 1 30); do
  wk="$(curl -sS -H 'Accept: application/json' "$BASE/.well-known/nodeinfo" || true)"
  if jq -e '.links[] | select(.href | endswith("/nodeinfo/2.0"))' <<<"$wk" >/dev/null 2>&1; then ok=1; break; fi
  sleep 2
done
[ -n "$ok" ] || { head -c 600 <<<"$wk"; fail "/.well-known/nodeinfo did not link nodeinfo 2.0"; }

page="$(curl -sS -L -w '\n%{http_code}' "$BASE/login" || true)"
[ "$(tail -n1 <<<"$page")" = "200" ] || { head -30 <<<"$page"; fail "/login did not serve HTTP 200"; }
grep -q 'name="password"' <<<"$page" || { head -40 <<<"$page"; fail "/login did not render the login form"; }

# The swapped jQuery (with Migrate appended) is what the templates load.
js="$(curl -sS "$BASE/view/asset/jquery/dist/jquery.min.js" || true)"
grep -q 'jQuery v3\.' <<<"$js" || fail "jquery.min.js is not jQuery 3"
grep -q 'jQuery Migrate' <<<"$js" || fail "jquery.min.js does not carry jQuery Migrate"

for p in config/local.config.php vendor/autoload.php bin/console.php src/App.php; do
  c="$(curl -sS -o /dev/null -w '%{http_code}' "$BASE/$p" || true)"
  [ "$c" = "403" ] || fail "/$p is reachable over HTTP (got $c)"
done

# The worker loop retries every 30 s; it was waiting for local.config.php until now. The
# daemon runs in the foreground (no pidfile), so prove it by work done: jobs the install
# queued must get processed by bin/worker.php processes it spawns.
done_jobs=0
for _ in $(seq 1 40); do
  done_jobs="$(docker exec "$DB" mariadb -ufriendica -pfriendicapw -N -e 'select count(*) from workerqueue where done = 1' friendica 2>/dev/null || echo 0)"
  [ "${done_jobs:-0}" -gt 0 ] && break
  sleep 3
done
[ "${done_jobs:-0}" -gt 0 ] || fail "the worker daemon processed no queued job"
echo "worker: $done_jobs queued jobs done"

if docker logs "$NAME" 2>&1 | grep -E 'PHP (Fatal|Parse) error|Uncaught'; then fail "php logged a fatal error"; fi
echo "smoke test passed (friendica ${WANT:-?}, uid $user, installed, nodeinfo + login + worker)"
