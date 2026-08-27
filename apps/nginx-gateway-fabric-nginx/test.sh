#!/usr/bin/env bash
# Smoke test for a built NGINX Gateway Fabric data-plane image.
# Usage: test.sh <image-ref> [expected-ngf-version]
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
NAME="quench-ngf-nginx-smoke-$$"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

run() { docker run --rm --entrypoint "$1" "$IMAGE" "${@:2}"; }

# 1) nginx exists, reports a real version, and carries the modules NGF's generated
#    config needs. stream_ssl_preread backs TLSRoute; njs is non-optional.
nginx_ver="$(run /usr/bin/nginx -v 2>&1 | sed -n 's|.*nginx/||p')"
echo "nginx: $nginx_ver"
case "$nginx_ver" in ""|*[!0-9.]*) echo "nginx version not reported: '$nginx_ver'"; exit 1 ;; esac
conf="$(run /usr/bin/nginx -V 2>&1)"
for flag in --with-stream_ssl_preread_module --with-http_ssl_module --with-http_v2_module --with-compat; do
  printf '%s' "$conf" | grep -q -- "$flag" || { echo "nginx built without $flag"; exit 1; }
done
printf '%s' "$conf" | grep -q -- '--add-dynamic-module=../njs' \
  || { echo "nginx built without the njs dynamic module"; exit 1; }

# 2) nginx-agent exists and is stamped. A wrong/absent agent is the failure that
#    presents as a Gateway that never programs, with nothing in the nginx log.
agent_ver="$(run /usr/bin/nginx-agent --version 2>&1 | tr -d '\r')"
echo "nginx-agent: $agent_ver"
printf '%s' "$agent_ver" | grep -qE '[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "nginx-agent version not stamped: '$agent_ver'"; exit 1; }

# 3) THE test: NGF's own nginx.conf must parse. It opens with
#    `load_module modules/ngx_http_js_module.so;` and js_import's two njs scripts, so a
#    pass proves the module was built, the /etc/nginx/modules symlink resolves and the
#    scripts are where nginx.conf expects them. This is the difference between a green
#    scan and an image that can actually serve a Gateway.
run /usr/bin/nginx -t -c /etc/nginx/nginx.conf 2>&1 | tail -2
run /usr/bin/nginx -t -c /etc/nginx/nginx.conf >/dev/null 2>&1 \
  || { echo "NGF nginx.conf failed to parse"; exit 1; }

# 4) The grpc error pages the generated gRPC servers include must be present.
run /bin/sh -c 'test -f /etc/nginx/grpc-error-locations.conf && test -f /etc/nginx/grpc-error-pages.conf'

# 5) The entrypoint really does supervise BOTH processes. nginx-agent refuses to start
#    without a config (in the pod an init container writes one from a ConfigMap), so the
#    boot test writes a minimal one and then runs the real entrypoint. What is asserted
#    is what the pod needs: the container stays up and both processes are alive.
#    /etc/nginx-agent is owned 101:1001 in the image precisely so this works.
docker run -d --name "$NAME" --entrypoint /bin/sh "$IMAGE" -c '
  printf "log:\n  level: info\n  path: /var/log/nginx-agent/\nallowed_directories:\n  - /etc/nginx\n  - /var/run/nginx\n" \
    > /etc/nginx-agent/nginx-agent.conf
  exec /usr/bin/docker-entrypoint.sh' >/dev/null
sleep 10
state="$(docker inspect "$NAME" --format '{{.State.Status}}')"
procs="$(docker top "$NAME" -eo args 2>/dev/null || docker top "$NAME")"
printf '%s\n' "$procs"
docker logs "$NAME" 2>&1 | tail -5
[ "$state" = "running" ] || { echo "container is $state, the supervisor died"; exit 1; }
# Match the ARGV, not nginx's own process title ("nginx: master process ..."): when the
# arm64 image is booted on an amd64 host the qemu wrapper is what `docker top` reports,
# and the rewritten title never shows up. Both forms contain the binary path.
printf '%s' "$procs" | grep -q '/usr/bin/nginx -g' \
  || { echo "nginx master is not running under the entrypoint"; exit 1; }
printf '%s' "$procs" | grep -q '/usr/bin/nginx-agent' \
  || { echo "nginx-agent is not running under the entrypoint"; exit 1; }
echo "entrypoint supervises nginx + nginx-agent"

# The pid file NGF's nginx.conf declares must actually be there -- the entrypoint waits
# on it before starting the agent, so a wrong path would hang the container for 30s and
# then exit.
docker exec "$NAME" test -f /var/run/nginx/nginx.pid \
  || { echo "no pid file at /var/run/nginx/nginx.pid"; exit 1; }

# 6) uid 101, because that is what the provisioner pins the container to.
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "101" ] || { echo "expected user 101, got '$user'"; exit 1; }

echo "smoke test passed (nginx $nginx_ver, agent stamped, NGF nginx.conf parses, user $user)"
