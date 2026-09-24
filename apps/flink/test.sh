#!/usr/bin/env bash
# Smoke test for a built flink image. Usage: test.sh <image-ref> <version>
# Starts a jobmanager and a taskmanager on a private network, then requires the REST API
# to report the version and one registered taskmanager with free slots.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> <version>}"
VERSION="${2:?usage: test.sh <image-ref> <version>}"
NET="flink-smoke-$$"; JM="flink-jm-$$"; TM="flink-tm-$$"
cleanup() { docker rm -f "$JM" "$TM" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

docker network create "$NET" >/dev/null
docker run -d --name "$JM" --network "$NET" --hostname jobmanager --read-only --tmpfs /tmp \
  -e JOB_MANAGER_RPC_ADDRESS=jobmanager -p 127.0.0.1:18081:8081 "$IMAGE" jobmanager >/dev/null
docker run -d --name "$TM" --network "$NET" --read-only --tmpfs /tmp \
  -e JOB_MANAGER_RPC_ADDRESS=jobmanager -e TASK_MANAGER_NUMBER_OF_TASK_SLOTS=2 "$IMAGE" taskmanager >/dev/null

ok=0
for _ in $(seq 1 90); do
  out="$(curl -fsS http://127.0.0.1:18081/overview 2>/dev/null || true)"
  if echo "$out" | grep -q '"taskmanagers":1'; then ok=1; break; fi
  sleep 1
done
[ "$ok" = 1 ] || { echo "no taskmanager registered: $out"; docker logs "$JM" 2>&1 | tail -30; docker logs "$TM" 2>&1 | tail -30; exit 1; }
echo "$out"
echo "$out" | grep -q "\"flink-version\":\"${VERSION}\"" || { echo "version ${VERSION} not reported"; exit 1; }
echo "$out" | grep -q '"slots-available":2' || { echo "expected 2 free slots"; exit 1; }
echo "smoke test passed (nonroot $user, flink ${VERSION}, read-only root, 1 taskmanager with 2 slots)"
