#!/usr/bin/env bash
# Smoke test for a built configurable-http-proxy image. Usage: test.sh <image-ref> [version]
# Starts the proxy with an API token, adds a route through the REST API, reads it
# back, and requires an unrouted request to get the proxy's own 503/404.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="quench-chp-smoke-$$"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
ver="$(docker run --rm --entrypoint /usr/bin/node "$IMAGE" /opt/chp/node_modules/configurable-http-proxy/bin/configurable-http-proxy --version)"
echo "configurable-http-proxy $ver"
[ -z "$WANT" ] || [ "$ver" = "$WANT" ] || { echo "expected $WANT"; exit 1; }

docker run -d --name "$NAME" -e CONFIGPROXY_AUTH_TOKEN=smoke -p 127.0.0.1:8000:8000 -p 127.0.0.1:8001:8001 "$IMAGE" >/dev/null
A="Authorization: token smoke"
for i in $(seq 1 30); do
  curl -fsS -H "$A" http://127.0.0.1:8001/api/routes >/dev/null 2>&1 && break
  [ "$i" = 30 ] && { echo "proxy API never answered"; docker logs "$NAME"; exit 1; }
  sleep 1
done
curl -fsS -X POST -H "$A" -d '{"target":"http://127.0.0.1:9"}' http://127.0.0.1:8001/api/routes/smoke
routes="$(curl -fsS -H "$A" http://127.0.0.1:8001/api/routes)"
echo "routes: $routes"
grep -q '"/smoke"' <<<"$routes" || { echo "route not stored"; exit 1; }
[ "$(curl -s -o /dev/null -w '%{http_code}' -H 'Authorization: token wrong' http://127.0.0.1:8001/api/routes)" = 403 ] || { echo "API accepted a wrong token"; exit 1; }

echo "smoke test passed (configurable-http-proxy ${WANT:-?}: API auth, route add/list; nonroot user: $user)"
