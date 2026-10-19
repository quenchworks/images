#!/usr/bin/env bash
# Smoke test for a built Gravitee console image. Usage: test.sh <image-ref> [expected-version]
#
# Serves the console on a read-only root: index.html is the Gravitee console, a hashed
# JS chunk comes back as JavaScript, constants.json points at the management API, and
# an SPA deep link falls back to index.html.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
NAME="quench-gravitee-console-smoke-$$"
PORT=18380
B="http://127.0.0.1:$PORT"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT
fail() { echo "FAIL: $*"; docker logs "$NAME" 2>&1 | tail -30; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

docker run -d --name "$NAME" --read-only --tmpfs /tmp:rw,mode=1777 -p "127.0.0.1:$PORT:8080" "$IMAGE" >/dev/null
for i in $(seq 1 30); do
  curl -fsS "$B/" >/dev/null 2>&1 && break
  [ "$i" = 30 ] && fail "nginx did not start serving"
  sleep 1
done

body="$(curl -fsS "$B/")"
grep -qi 'gravitee' <<<"$body" || fail "index.html does not look like the Gravitee console"
js="$(grep -oE 'main\.[0-9a-f]+\.js' <<<"$body" | head -1 || true)"
[ -n "$js" ] || fail "no main.<hash>.js in index.html"
ct="$(curl -fsSI "$B/$js" | tr -d '\r' | awk -F': ' 'tolower($1)=="content-type"{print $2}')"
case "$ct" in *javascript*) ;; *) fail "$js served as '$ct'";; esac
consts="$(curl -fsS "$B/constants.json")"
grep -q '"baseURL" *: *"/management"' <<<"$consts" || fail "constants.json is '$consts'"
deep="$(curl -fsS "$B/some/deep/console/route")"
[ "$deep" = "$body" ] || fail "SPA deep link did not fall back to index.html"
echo "smoke test passed (uid $user, console index, $js as $ct, constants.json, SPA fallback)"
