#!/usr/bin/env bash
# Smoke test for a built wiremock image. Usage: test.sh <image-ref> [version]
# Real mock-server work on a read-only root: a stub file mounted into mappings/ serves a
# handlebars-templated response (the pinned handlebars core with the 4.3 helpers), a
# stub posted to the admin API proxies back through Jetty 12 and httpclient5, HTTPS
# answers over HTTP/2, and the request journal records the calls.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="wiremock-smoke-$$"
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
trap 'docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$WORK"' EXIT
trap 'echo "failed at line $LINENO"; docker logs "$NAME" 2>&1 | tail -30' ERR

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

mkdir -p "$WORK/mappings"
cat > "$WORK/mappings/hello.json" <<'JSON'
{"request":{"method":"GET","urlPath":"/hello"},
 "response":{"status":200,"transformers":["response-template"],
   "body":"hi {{request.query.name}} {{numberFormat 1234.5 '#,##0.00'}} {{math 2 '+' 3}}"}}
JSON
chmod -R a+rX "$WORK"
docker run -d --name "$NAME" --read-only --tmpfs /tmp -v "$WORK/mappings:/home/wiremock/mappings:ro" \
  -p 127.0.0.1:18080:8080 -p 127.0.0.1:18443:8443 "$IMAGE" --https-port 8443 >/dev/null
B=http://127.0.0.1:18080
ok=0
for _ in $(seq 1 60); do curl -fsS "$B/__admin/health" 2>/dev/null | grep -q '"status" *: *"healthy"' && { ok=1; break; }; sleep 1; done
[ "$ok" = 1 ] || { echo "wiremock never reported healthy"; docker logs "$NAME" 2>&1 | tail -30; exit 1; }
[ -z "$WANT" ] || curl -fsS "$B/__admin/health" | grep -q "\"version\" *: *\"$WANT\"" || { echo "version $WANT not reported"; exit 1; }

test "$(curl -fsS "$B/hello?name=quench")" = "hi quench 1,234.50 5"
curl -fsS -o /dev/null -X POST -d '{"request":{"method":"GET","urlPathPattern":"/via/.*"},"response":{"proxyBaseUrl":"http://127.0.0.1:8080","proxyUrlPrefixToRemove":"/via"}}' "$B/__admin/mappings"
test "$(curl -fsS "$B/via/hello?name=proxied")" = "hi proxied 1,234.50 5"
test "$(curl -sSk -o /dev/null -w '%{http_code} %{http_version}' --http2 https://127.0.0.1:18443/__admin/health)" = "200 2"
n="$(curl -fsS "$B/__admin/requests" | grep -o '"url" *: *"/[^"]*hello' | wc -l)"
[ "$n" -ge 2 ] || { echo "journal recorded $n hello calls"; exit 1; }
if docker logs "$NAME" 2>&1 | grep -E 'Exception|ERROR'; then echo "wiremock logged errors"; exit 1; fi
echo "smoke test passed (wiremock ${WANT:-?}, uid $user, templating, proxy, HTTPS/2, journal)"
