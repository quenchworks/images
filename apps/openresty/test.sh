#!/usr/bin/env bash
# Smoke test for a built OpenResty image. Usage: test.sh <image-ref> [version]
# Runs it read-only with a tmpfs /tmp, serves the default page and stub_status, and
# mounts a Lua server block: LuaJIT must answer, and resty.sha256 (FFI into the
# libcrypto nginx links) must hash correctly. That proves lua-nginx-module,
# resty.core and the OpenSSL linkage, not just a listening port.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="quench-openresty-smoke-$$"
CONF="$(mktemp -d)"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$CONF"; }
trap cleanup EXIT
fail() { echo "$1"; docker logs "$NAME" 2>&1 | tail -30; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

ver="$(docker run --rm "$IMAGE" -v 2>&1 | sed -n 's|.*openresty/\([0-9.]*\).*|\1|p')"
echo "openresty version: $ver"
[ -n "$ver" ] || { echo "no version from -v"; exit 1; }
[ -z "$WANT" ] || [ "$ver" = "$WANT" ] || { echo "expected $WANT"; exit 1; }

cat > "$CONF/lua.conf" <<'CONF'
server {
    listen 8081;
    location = /lua {
        default_type text/plain;
        content_by_lua_block { ngx.say("lua ok ", jit.version) }
    }
    location = /sha {
        default_type text/plain;
        content_by_lua_block {
            local sha = require("resty.sha256"):new()
            sha:update("quench")
            ngx.say(require("resty.string").to_hex(sha:final()))
        }
    }
}
CONF
chmod 0755 "$CONF"; chmod 0644 "$CONF/lua.conf"

docker run -d --name "$NAME" --read-only --tmpfs /tmp:rw,mode=1777 \
  -v "$CONF/lua.conf:/etc/nginx/conf.d/lua.conf:ro" \
  -p 127.0.0.1:8080:8080 -p 127.0.0.1:8081:8081 "$IMAGE" >/dev/null

for i in $(seq 1 30); do
  curl -fsS http://127.0.0.1:8080/ >/dev/null 2>&1 && break
  [ "$i" = 30 ] && fail "openresty did not become ready"
  sleep 1
done
grep -qi 'openresty' <<<"$(curl -fsS http://127.0.0.1:8080/)" || fail "default page missing"
grep -q 'Active connections' <<<"$(curl -fsS http://127.0.0.1:8080/stub_status)" || fail "stub_status failed"

lua="$(curl -fsS http://127.0.0.1:8081/lua)"
echo "$lua"
grep -q '^lua ok LuaJIT 2\.1' <<<"$lua" || fail "Lua handler did not run"
sha="$(curl -fsS http://127.0.0.1:8081/sha)"
[ "$sha" = "a8b51e95fe15708a5f253f567e72f00f052cd6c11f013b19c5b122bc52b98073" ] || fail "resty.sha256 returned '$sha'"

if docker logs "$NAME" 2>&1 | grep -qiE 'read-only file system|permission denied|\[emerg\]|\[alert\]'; then
  fail "errors in the log"
fi
echo "smoke test passed (openresty $ver, LuaJIT + resty.sha256 ok, nonroot user: $user)"
