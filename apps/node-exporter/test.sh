#!/usr/bin/env bash
# Smoke test for a built node_exporter image. Usage: test.sh <image-ref>
# Runs read-only rootfs (the posture the chart ships), waits for /metrics, then
# asserts / and /metrics return 200 and the version is stamped. Confirms the
# container runs as nonroot uid 1001.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-node-exporter-smoke-$$"
BASE="http://127.0.0.1:9100"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "version check:"
docker run --rm "$IMAGE" --version

echo "starting $IMAGE (read-only rootfs)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /tmp:rw,mode=1777 \
  -p 127.0.0.1:9100:9100 \
  "$IMAGE" >/dev/null

# wait for /metrics to come up
for i in $(seq 1 30); do
  if curl -fsS "$BASE/metrics" >/dev/null 2>&1; then break; fi
  [ "$i" = 30 ] && { echo "node_exporter did not serve /metrics"; docker logs "$NAME"; exit 1; }
  sleep 1
done

echo "checking / returns 200"
curl -fsS "$BASE/" >/dev/null || { echo "/ did not return 200"; docker logs "$NAME"; exit 1; }

echo "checking /metrics exposes node_exporter_build_info"
# capture fully first: /metrics is large, a streaming `grep -q` closes the pipe
# early and makes curl report a write failure.
metrics="$(curl -fsS "$BASE/metrics" 2>/dev/null || true)"
# here-string, not a pipe: under `set -o pipefail`, `grep -q` exits early and
# SIGPIPEs the upstream printf, making a real match look like a failure.
grep -q 'node_exporter_build_info' <<<"$metrics" \
  || { echo "node_exporter_build_info not found on /metrics"; docker logs "$NAME"; exit 1; }

# must run as the nonroot exporter user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot uid: $user, read-only rootfs, / + /metrics 200)"
