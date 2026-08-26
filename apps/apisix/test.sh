#!/usr/bin/env bash
# Smoke test for a built Apache APISIX image. Usage: test.sh <image-ref> [expected-version]
#
# A REAL functional check, not a TCP accept. APISIX can start, answer a health port and
# still proxy nothing -- so this declares a route in standalone (file-driven) mode and
# proves a request actually traverses the proxy port to a separate upstream container.
#
# NO ETCD SIDECAR. APISIX normally needs etcd, but 3.18 keeps the file-driven standalone
# mode (deployment.role=data_plane + role_data_plane.config_provider=yaml, documented in
# docs/en/latest/deployment-modes.md), which reads conf/apisix.yaml and never contacts
# etcd. That is what the image ships by default, so a genuine boot test needs one
# container. Standalone also re-reads conf/apisix.yaml once a second, which is how the
# route below is injected after start.
#
# It also asserts the things this recipe specifically controls, each of which could
# regress silently into a "0 CVEs but subtly broken" image:
#   1) the rebuilt runtime really carries api7's modules -- without
#      "apisix-nginx-module" in `openresty -V`, apisix/cli/env.lua flips
#      use_apisix_base off and generates a DEGRADED nginx.conf that still starts
#   2) not one plugin failed to load. Plugin loading is pcall'd and only LOGGED, so a
#      missing rock (lua-resty-saml is the fragile one) is invisible unless asserted
#   3) the admin SPA really got built into /usr/local/apisix/ui
#   4) nonroot 1001 on a read-only rootfs with only conf/ + logs/ writable
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
WANT_VER="${2:-}"
C="quench-apisix-$$"
UP="quench-apisix-upstream-$$"
NET="quench-apisix-net-$$"
PROXY=19080

cleanup() {
  docker rm -f "$C" "$UP" >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
}
trap cleanup EXIT

# --- nonroot uid 1001 ---
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

docker network create "$NET" >/dev/null

# A real upstream for the proxy to reach. APISIX's own control port is not a
# substitute: proxying to it would not prove a route was matched and forwarded.
# nginx:alpine rather than a hand-rolled `nc` listener -- nc produces a malformed
# response and the gateway then reports an upstream error, which looks like a routing
# failure but is actually the opposite.
docker run -d --name "$UP" --network "$NET" nginx:alpine >/dev/null 2>&1 \
  || { echo "could not start the test upstream"; exit 1; }
for _ in $(seq 1 20); do
  docker exec "$UP" sh -c 'wget -qO- http://127.0.0.1/ >/dev/null 2>&1' && break
  sleep 1
done

echo "== starting APISIX standalone (read-only rootfs, writable conf/ + logs/) =="
# conf/ and logs/ are tmpfs, exactly as the chart backs them with an emptyDir. They are
# shipped EMPTY and the entrypoint re-seeds conf/ from the read-only conf.default/, so
# this also tests that seeding -- if it regressed, nginx dies on conf/mime.types.
docker run -d --name "$C" --network "$NET" \
  --read-only \
  --tmpfs /tmp:rw,mode=1777 \
  --tmpfs /usr/local/apisix/conf:rw,mode=0755,uid=1001,gid=1001 \
  --tmpfs /usr/local/apisix/logs:rw,mode=0755,uid=1001,gid=1001 \
  -p "$PROXY:9080" \
  "$IMAGE" >/dev/null

# --- the gateway must answer on the proxy port ---
# With no routes loaded yet APISIX answers 404 with its own Server header, which is a
# positive signal (the Lua stack ran) rather than a connection refusal.
code=""
for _ in $(seq 1 90); do
  code="$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$PROXY/" || true)"
  [ -n "$code" ] && [ "$code" != "000" ] && break
  sleep 1
done
if [ -z "$code" ] || [ "$code" = "000" ]; then
  echo "APISIX never answered on the proxy port. Container log:"
  docker logs "$C" 2>&1 | tail -50
  docker exec "$C" sh -c 'tail -50 /usr/local/apisix/logs/error.log' 2>&1 || true
  exit 1
fi

server="$(curl -sI "http://127.0.0.1:$PROXY/" | tr -d '\r' | sed -n 's/^[Ss]erver: //p')"
echo "  proxy answered $code, Server: ${server:-<none>}"
case "$server" in
  APISIX/*) : ;;
  *) echo "FAIL: the Server header is not APISIX's ('$server')"; docker logs "$C" 2>&1 | tail -30; exit 1 ;;
esac

if [ -n "$WANT_VER" ]; then
  [ "$server" = "APISIX/$WANT_VER" ] \
    || { echo "FAIL: expected APISIX/$WANT_VER, got '$server'"; exit 1; }
  echo "  version matches $WANT_VER"
fi

# --- the rebuilt runtime really carries api7's modules ---
# Ask the binary what it was configured WITH rather than grepping for internal symbol
# names -- those are each module's implementation detail. This is the SAME test
# apisix/cli/env.lua uses to decide use_apisix_base, so a miss here means the generated
# nginx.conf silently lost dynamic upstream keepalive, the `lua {}` meta subsystem,
# grpc :authority rewriting and the request-id log field.
echo "== runtime module check =="
CONF="$(docker exec "$C" /usr/local/openresty/bin/openresty -V 2>&1 || true)"
for m in apisix-nginx-module lua-resty-events lua-var-nginx-module ngx_http_ffi_client http_stub_status_module; do
  printf '%s' "$CONF" | grep -q -- "$m" \
    || { echo "FAIL: $m is not in the openresty configure line"; exit 1; }
done
printf '%s' "$CONF" | grep -q "APISIX_RUNTIME_VER=" \
  || { echo "FAIL: APISIX_RUNTIME_VER is not stamped into the runtime"; exit 1; }
echo "  apisix-nginx-module / events / ngxvar / ffi_client / stub_status all present"

# --- the admin SPA was built into the image ---
docker exec "$C" test -f /usr/local/apisix/ui/index.html \
  || { echo "FAIL: /usr/local/apisix/ui/index.html missing -- the dashboard stage did not run"; exit 1; }
echo "  admin SPA present ($(docker exec "$C" sh -c 'find /usr/local/apisix/ui -type f | wc -l' | tr -d ' ') files)"

# --- declare a route and prove traffic traverses the proxy ---
# Standalone file-driven mode polls conf/apisix.yaml every second, so writing it after
# start is a supported hot update. `#END` is mandatory: without it APISIX refuses to
# load the file at all (documented, and a silent no-op if forgotten).
echo "== declarative route + proxy round trip =="
docker exec -i "$C" sh -c 'cat > /usr/local/apisix/conf/apisix.yaml' <<YAML
routes:
  - uri: /echo
    # proxy-rewrite is load-bearing, not decoration. The upstream is nginx:alpine, which
    # serves "Welcome to nginx" at / and NOTHING at /echo, so proxying the path through
    # unchanged makes the UPSTREAM return its own 404 page. That reads as "traffic never
    # traversed the proxy" when in fact it traversed correctly and hit a path that does not
    # exist. Rewriting to / also proves a plugin actually executes on the request path,
    # which a bare upstream proxy would not.
    plugins:
      proxy-rewrite:
        uri: /
    upstream:
      nodes:
        "$UP:80": 1
      type: roundrobin
#END
YAML

body=""
for _ in $(seq 1 30); do
  body="$(curl -s "http://127.0.0.1:$PROXY/echo" || true)"
  case "$body" in *"Welcome to nginx"*) break ;; esac
  sleep 1
done
case "$body" in
  *"Welcome to nginx"*) echo "  proxy round trip OK" ;;
  *)
    echo "FAIL: request did not traverse the proxy to the upstream (got: '${body:0:120}')"
    docker exec "$C" sh -c 'tail -40 /usr/local/apisix/logs/error.log' 2>&1 || true
    exit 1 ;;
esac

# --- not one plugin failed to load ---
# APISIX pcall()s every plugin require and only LOGS a failure, so a missing rock
# (lua-resty-saml, which needs a hand-built xmlsec1, is the fragile one and saml-auth
# IS in the default plugin list) leaves a working-looking gateway with a silently
# disabled plugin. Assert zero failures instead.
echo "== plugin load check =="
# saml-auth is the ONE expected failure: we deliberately do not ship api7's
# lua-resty-saml, because it statically links a 2019 xmlsec fork that Trivy cannot
# inspect (see the "Stage 2 REMOVED" block in melange.yaml). It is still in APISIX's
# default plugin list, so it logs a load failure on every start.
#
# This asserts saml-auth is the ONLY failure, and separately that it IS failing. The
# second half matters: if saml-auth ever loads cleanly, the removal silently stopped
# working and we are shipping the unscannable static library again.
expected_fail="saml-auth"
failed="$(docker exec "$C" sh -c 'grep "failed to load plugin" /usr/local/apisix/logs/error.log 2>/dev/null | grep -cv "'"$expected_fail"'" || echo 0' | tr -d ' \r')"
saml_failed="$(docker exec "$C" sh -c 'grep -c "failed to load plugin.*'"$expected_fail"'" /usr/local/apisix/logs/error.log 2>/dev/null || echo 0' | tr -d ' \r')"
if [ "${saml_failed:-0}" -eq 0 ]; then
  echo "FAIL: $expected_fail loaded, but this image deliberately omits lua-resty-saml."
  echo "      Either the removal regressed, or the plugin is no longer in the default list."
  exit 1
fi
if [ "${failed:-0}" != "0" ]; then
  echo "FAIL: $failed plugin(s) failed to load:"
  docker exec "$C" sh -c 'grep "failed to load plugin" /usr/local/apisix/logs/error.log | head -10'
  exit 1
fi
loaded="$(docker exec "$C" sh -c 'grep -o "load(): new plugins: {[^}]*}" /usr/local/apisix/logs/error.log | head -1 | grep -o "\"[a-z0-9-]*\":true" | wc -l' | tr -d ' \r')"
echo "  0 unexpected plugin load failures (saml-auth omitted by design), ${loaded:-?} http plugins loaded"
[ "${loaded:-0}" -ge 100 ] \
  || { echo "FAIL: only ${loaded:-0} plugins loaded, expected >=100 -- the rock tree is incomplete"; exit 1; }

# --- nonroot, for real, inside the container ---
uid="$(docker exec "$C" sh -c 'awk "/^Uid:/{print \$2; exit}" /proc/1/status' | tr -d ' \r')"
[ "$uid" = "1001" ] || { echo "FAIL: pid 1 runs as uid $uid, not 1001"; exit 1; }

echo "smoke test passed (nonroot 1001, read-only rootfs, ${server}, api7 modules linked, admin SPA present, ${loaded} plugins, proxy round trip OK)"
