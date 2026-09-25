#!/usr/bin/env bash
# Smoke test for a built zipkin image. Usage: test.sh <image-ref> [version]
# Real tracing work: the server starts on a read-only root, accepts a span on the v2
# API, lists its service, returns it by trace ID, reports its version and serves the UI.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="zipkin-smoke-$$"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

docker run -d --name "$NAME" --read-only --tmpfs /tmp -p 127.0.0.1:19411:9411 "$IMAGE" >/dev/null
ok=0
for _ in $(seq 1 120); do
  curl -fsS http://127.0.0.1:19411/health 2>/dev/null | grep -q '"status" *: *"UP"' && { ok=1; break; }
  sleep 1
done
[ "$ok" = 1 ] || { echo "zipkin never reported UP"; docker logs "$NAME" 2>&1 | tail -40; exit 1; }

info="$(curl -fsS http://127.0.0.1:19411/info)"; echo "$info"
[ -z "$WANT" ] || echo "$info" | grep -q "\"version\":\"${WANT}\"" || { echo "expected version $WANT"; exit 1; }

code="$(curl -s -o /dev/null -w '%{http_code}' -X POST -H 'Content-Type: application/json' http://127.0.0.1:19411/api/v2/spans \
  -d '[{"traceId":"5af7183fb1d4cf5f","id":"6b221d5bc9e6496c","name":"get /","timestamp":1790000000000000,"duration":2000,"localEndpoint":{"serviceName":"quench-smoke"}}]')"
[ "$code" = 202 ] || { echo "span POST returned $code"; exit 1; }
for _ in $(seq 1 20); do curl -fsS http://127.0.0.1:19411/api/v2/services | grep -q quench-smoke && break; sleep 1; done
curl -fsS http://127.0.0.1:19411/api/v2/services | grep -q quench-smoke || { echo "service not listed"; exit 1; }
curl -fsS http://127.0.0.1:19411/api/v2/trace/5af7183fb1d4cf5f | grep -q '"id":"6b221d5bc9e6496c"' || { echo "trace not returned"; exit 1; }
curl -fsS http://127.0.0.1:19411/zipkin/ | grep -qi '<html' || { echo "UI not served"; exit 1; }
if docker logs "$NAME" 2>&1 | grep -E ' ERROR |Exception'; then echo "server logged errors"; exit 1; fi
echo "smoke test passed (zipkin ${WANT:-?}, uid $user, span stored and queried)"
