#!/usr/bin/env bash
# Smoke test for a built Pushgateway image. Usage: test.sh <image-ref> [version]
# A job pushes a metric, which /metrics must then expose with its job label; deleting the
# group must remove it again. Health and readiness endpoints must answer.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="pushgateway-smoke-$$"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
v="$(docker run --rm "$IMAGE" --version 2>&1)"
echo "$v" | head -1
[ -z "$WANT" ] || grep -qF "version $WANT" <<<"$v" || { echo "expected $WANT"; exit 1; }

docker run -d --name "$NAME" --read-only -p 127.0.0.1:19091:9091 "$IMAGE" >/dev/null
for _ in $(seq 1 30); do curl -fsS http://127.0.0.1:19091/-/ready >/dev/null 2>&1 && break; sleep 1; done
curl -fsS http://127.0.0.1:19091/-/healthy >/dev/null
printf 'quench_smoke_value 42\n' | curl -fsS --data-binary @- http://127.0.0.1:19091/metrics/job/quench_smoke
m="$(curl -fsS http://127.0.0.1:19091/metrics)"
grep -qE '^quench_smoke_value\{.*job="quench_smoke".*\} 42$' <<<"$m" || { echo "pushed metric not exposed"; grep quench_smoke <<<"$m"; exit 1; }
echo "pushed and scraped: $(grep '^quench_smoke_value' <<<"$m")"
curl -fsS -X DELETE http://127.0.0.1:19091/metrics/job/quench_smoke
m="$(curl -fsS http://127.0.0.1:19091/metrics)"
if grep -q '^quench_smoke_value' <<<"$m"; then echo "deleted group still exposed"; exit 1; fi
if docker logs "$NAME" 2>&1 | grep -qE 'panic|read-only file system'; then echo "crashed or wrote to the root"; exit 1; fi
echo "smoke test passed: push, scrape, delete, health, user 1001"
