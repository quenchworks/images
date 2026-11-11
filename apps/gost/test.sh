#!/usr/bin/env bash
# Smoke test for a built GOST image. Usage: test.sh <image-ref> [version]
# Runs an authenticated HTTP proxy and fetches GOST's own API through it (so the proxy path,
# auth and the API service are all exercised), checks a request without credentials gets 407,
# and that the metrics service answers.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="gost-smoke-$$"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
v="$(docker run --rm "$IMAGE" -V)"
echo "$v"
[ -z "$WANT" ] || grep -qF "$WANT" <<<"$v" || { echo "expected $WANT"; exit 1; }

docker run -d --name "$NAME" --read-only -p 127.0.0.1:18080:8080 -p 127.0.0.1:19000:9000 "$IMAGE" \
  -L 'http://smoke:smoke-pass@:8080' -api :18081 -metrics :9000 >/dev/null

ok=0
for _ in $(seq 1 30); do
  out="$(curl -fsS -x http://smoke:smoke-pass@127.0.0.1:18080 http://127.0.0.1:18081/config 2>/dev/null || true)"
  grep -q '"services"' <<<"$out" && { ok=1; break; }
  sleep 1
done
[ "$ok" = 1 ] || { echo "API not reachable through the proxy"; docker logs "$NAME" 2>&1 | tail -20; exit 1; }
echo "proxy ok: fetched the gost API config through the authenticated HTTP proxy"
code="$(curl -s -o /dev/null -w '%{http_code}' -x http://127.0.0.1:18080 http://127.0.0.1:18081/config)"
[ "$code" = 407 ] || { echo "no credentials got HTTP $code"; exit 1; }
m="$(curl -fsS http://127.0.0.1:19000/metrics)"
grep -q 'gost_\|go_goroutines' <<<"$m" || { echo "no metrics"; exit 1; }
echo "smoke test passed: proxy, auth refusal (407), API, metrics, user 1001"
