#!/usr/bin/env bash
# Smoke test for a built Kong Gateway image. Usage: test.sh <image-ref>
#
# A REAL functional check, not a TCP accept. Kong can start, answer its Admin API, and
# still proxy nothing -- so this declares a route through the Admin API and then proves
# a request actually traverses the proxy port to an upstream.
#
# It also asserts the things this recipe specifically controls, each of which could
# regress silently:
#   1) the patched nginx really has Kong's modules compiled in
#   2) the router FFI library (atc-router) loads -- a missing libatc_router.so shows up
#      as routes that never match rather than as a startup failure
#   3) nonroot 1001 on a read-only rootfs with a writable KONG_PREFIX
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
C="quench-kong-$$"
UP="quench-kong-upstream-$$"
NET="quench-kong-net-$$"
PROXY=18000
ADMIN=18001

cleanup() {
  docker rm -f "$C" "$UP" >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
}
trap cleanup EXIT

command -v jq >/dev/null || { echo "jq is required"; exit 1; }

# --- nonroot uid 1001 ---
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

docker network create "$NET" >/dev/null

# A real upstream for the proxy to reach. Kong's own /status is not a substitute:
# proxying to it would not prove a route was matched and forwarded.
#
# nginx:alpine rather than a hand-rolled `nc` listener -- the nc version produced a
# malformed response and Kong answered "An invalid response was received from the
# upstream server", which looks like a routing failure but is actually the opposite
# (Kong reached the upstream and rejected ITS reply).
docker run -d --name "$UP" --network "$NET" nginx:alpine >/dev/null 2>&1 \
  || { echo "could not start the test upstream"; exit 1; }
for i in $(seq 1 20); do
  docker exec "$UP" sh -c 'wget -qO- http://127.0.0.1/ >/dev/null 2>&1' && break
  sleep 1
done

echo "== starting Kong DB-less (read-only rootfs, writable prefix) =="
docker run -d --name "$C" --network "$NET" \
  --read-only \
  --tmpfs /tmp:rw,mode=1777 \
  --tmpfs /kong_prefix:rw,mode=0755,uid=1001,gid=1001 \
  -p "$PROXY:8000" -p "$ADMIN:8001" \
  "$IMAGE" >/dev/null

# --- Admin API must answer ---
code=""
for i in $(seq 1 60); do
  code="$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$ADMIN/" || true)"
  [ "$code" = "200" ] && break
  sleep 1
done
if [ "$code" != "200" ]; then
  echo "Admin API never returned 200 (last=$code). Container log:"
  docker logs "$C" 2>&1 | tail -40
  exit 1
fi
info="$(curl -s "http://127.0.0.1:$ADMIN/")"
echo "kong version: $(echo "$info" | jq -r '.version')"
echo "$info" | jq -e '.version' >/dev/null || { echo "Admin API returned no version"; exit 1; }

# --- the patched nginx really carries Kong's modules ---
echo "== nginx module check =="
# Ask the binary what it was configured WITH rather than grepping for internal symbol
# names -- those are each module's implementation detail and guessing them produces false
# failures (lua-resty-events does not define ngx_lua_resty_events_module).
CONF="$(docker exec "$C" /usr/local/openresty/nginx/sbin/nginx -V 2>&1 || true)"
for m in lua-kong-nginx-module lua-resty-lmdb lua-resty-events; do
  printf '%s' "$CONF" | grep -q -- "$m" \
    || { echo "FAIL: $m is not in the nginx configure line"; exit 1; }
done
echo "  lua-kong-nginx-module / lmdb / events all present"

# --- the router FFI library is loadable ---
# It lives in a package.cpath dir, not /usr/local/kong/lib: resty/router/cdefs.lua
# resolves it by walking package.cpath only.
docker exec "$C" test -f /usr/local/lib/lua/5.1/libatc_router.so \
  || { echo "FAIL: libatc_router.so missing -- routes would never match"; exit 1; }

# --- declare a route and prove traffic traverses the proxy ---
# DB-less mode refuses Admin API writes, so reconfigure declaratively.
echo "== declarative config + proxy round trip =="
cat >"/tmp/kong-$$.yml" <<YAML
_format_version: "3.0"
services:
  - name: echo
    url: http://$UP:80
    routes:
      - name: echo-route
        paths:
          - /echo
        strip_path: true
YAML
curl -sS -X POST "http://127.0.0.1:$ADMIN/config" \
  -F "config=@/tmp/kong-$$.yml" -o "/tmp/kong-cfg-$$.json" -w 'POST /config -> %{http_code}\n'
rm -f "/tmp/kong-$$.yml"
grep -q '"services"' "/tmp/kong-cfg-$$.json" \
  || { echo "declarative config was rejected:"; head -c 600 "/tmp/kong-cfg-$$.json"; exit 1; }
rm -f "/tmp/kong-cfg-$$.json"

body=""
for i in $(seq 1 20); do
  body="$(curl -s "http://127.0.0.1:$PROXY/echo" || true)"
  [ -n "$body" ] && break
  sleep 1
done
echo "  proxy returned: '$body'"
case "$body" in
  *"Welcome to nginx"*) : ;;
  *) echo "FAIL: request did not traverse the proxy to the upstream"; docker logs "$C" 2>&1 | tail -25; exit 1 ;;
esac

echo "smoke test passed (nonroot $user, admin API up, kong nginx modules linked, atc-router present, proxy round trip OK)"
