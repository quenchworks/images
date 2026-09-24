#!/usr/bin/env bash
# Smoke test for a built prefect image. Usage: test.sh <image-ref> <version>
# Starts the server on a read-only root, checks health, version and the UI, then runs
# a real flow from a second container against the server's API.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> <version>}"
VERSION="${2:?usage: test.sh <image-ref> <version>}"
NAME="quench-prefect-smoke-$$"
NET="$NAME-net"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

docker network create "$NET" >/dev/null
docker run -d --name "$NAME" --network "$NET" --read-only --tmpfs /tmp \
  --tmpfs /var/lib/prefect:uid=1001,gid=1001 -p 127.0.0.1:14200:4200 "$IMAGE" >/dev/null
ok=0
for _ in $(seq 1 120); do
  curl -fsS http://127.0.0.1:14200/api/health 2>/dev/null | grep -q true && { ok=1; break; }
  sleep 1
done
[ "$ok" = 1 ] || { echo "server never became healthy"; docker logs "$NAME" 2>&1 | tail -40; exit 1; }

ver="$(curl -fsS http://127.0.0.1:14200/api/admin/version)"
echo "server version: $ver"
[ "$ver" = "\"${VERSION}\"" ] || { echo "expected ${VERSION}"; exit 1; }

curl -fsS http://127.0.0.1:14200/ | grep -qi '<html' || { echo "UI not served"; exit 1; }

# a real flow run, reported to the server
out="$(docker run --rm --network "$NET" --read-only --tmpfs /tmp --tmpfs /var/lib/prefect:uid=1001,gid=1001 \
  -e PREFECT_API_URL="http://$NAME:4200/api" --entrypoint /opt/prefect/venv/bin/python "$IMAGE" -c '
from prefect import flow, task
@task
def add(a, b): return a + b
@flow(name="quench-smoke")
def smoke(): return add(40, 2)
print("result", smoke())
' 2>&1)"
echo "$out" | grep -q 'result 42' || { echo "$out" | tail -30; exit 1; }
runs="$(curl -fsS -H 'Content-Type: application/json' -d '{}' http://127.0.0.1:14200/api/flow_runs/filter)"
echo "$runs" | grep -q '"state_type":"COMPLETED"' || { echo "no COMPLETED flow run on the server: $runs" | head -c 600; exit 1; }
echo "smoke test passed (nonroot $user, prefect ${VERSION}, flow run COMPLETED)"
