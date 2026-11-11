#!/usr/bin/env bash
# Smoke test for a built Skipper image. Usage: test.sh <image-ref> [version]
# Two skippers on one network: the backend answers with inlineContent, the front proxies to it
# by name, so the test proves routing, the reverse-proxy path and a filter (setResponseHeader),
# plus the support listener's /metrics and the eskip tool.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
ID="skipper-smoke-$$"
cleanup() { docker rm -f "$ID-b" "$ID-f" >/dev/null 2>&1 || true; docker network rm "$ID" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
v="$(docker run --rm "$IMAGE" -version)"
echo "$v"
[ -z "$WANT" ] || grep -qF "Skipper version v$WANT " <<<"$v" || { echo "expected v$WANT"; exit 1; }

docker network create "$ID" >/dev/null
docker run -d --name "$ID-b" --network "$ID" --read-only "$IMAGE" \
  -inline-routes 'r: * -> inlineContent("hello from backend") -> <shunt>' >/dev/null
docker run -d --name "$ID-f" --network "$ID" --read-only -p 127.0.0.1:19090:9090 -p 127.0.0.1:19911:9911 "$IMAGE" \
  -inline-routes "r: Path(\"/app\") -> setPath(\"/\") -> setResponseHeader(\"X-Quench\", \"ok\") -> \"http://$ID-b:9090\"" >/dev/null

ok=0
for _ in $(seq 1 30); do
  out="$(curl -fsS -D - http://127.0.0.1:19090/app 2>/dev/null || true)"
  grep -q 'hello from backend' <<<"$out" && { ok=1; break; }
  sleep 1
done
[ "$ok" = 1 ] || { echo "proxy route never answered"; docker logs "$ID-f" 2>&1 | tail -20; exit 1; }
grep -qi '^x-quench: ok' <<<"$out" || { echo "filter header missing"; echo "$out"; exit 1; }
code="$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:19090/nope)"
[ "$code" = 404 ] || { echo "unrouted path got HTTP $code"; exit 1; }
m="$(curl -fsS http://127.0.0.1:19911/metrics)"
grep -q 'skipper\|go_goroutines' <<<"$m" || { echo "no metrics"; exit 1; }
r="$(docker run --rm --entrypoint /usr/bin/eskip "$IMAGE" check -routes 'a: * -> <shunt>' 2>&1 || true)"
echo "eskip check: ${r:-ok}"
echo "smoke test passed: proxy route, filter, 404, metrics, user 1001"
