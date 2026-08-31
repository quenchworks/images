#!/usr/bin/env bash
# Smoke test for a built mongodb_exporter image. Usage: test.sh <image-ref> [version]
# Runs read-only rootfs, waits for /metrics on 9216, asserts exporter metrics are
# served and the container runs as nonroot uid 1001. Works without a MongoDB
# backend: the connection is made per scrape, so an unreachable URI just yields
# mongodb_up 0 instead of failing startup.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
EXPECT="${2:-}"
NAME="quench-mongodb-exporter-smoke-$$"
BASE="http://127.0.0.1:9216"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "version check:"
ver="$(docker run --rm "$IMAGE" --version 2>&1)"
echo "$ver"
if [ -n "$EXPECT" ]; then
  grep -q "Version: $EXPECT" <<<"$ver" || { echo "expected version $EXPECT in '--version' output"; exit 1; }
fi

echo "starting $IMAGE (read-only rootfs)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /tmp:rw,mode=1777 \
  -p 127.0.0.1:9216:9216 \
  "$IMAGE" --mongodb.uri=mongodb://127.0.0.1:27017 >/dev/null

# wait for /metrics to come up (a scrape against the absent mongod still returns 200)
for i in $(seq 1 30); do
  if curl -fsS --max-time 20 "$BASE/metrics" >/dev/null 2>&1; then break; fi
  [ "$i" = 30 ] && { echo "mongodb_exporter did not serve /metrics"; docker logs "$NAME"; exit 1; }
  sleep 1
done

echo "checking /metrics exposes exporter metrics"
# capture fully first: a streaming `grep -q` closes the pipe early and, under
# `set -o pipefail`, SIGPIPEs curl and makes a real match look like a failure.
metrics="$(curl -fsS --max-time 20 "$BASE/metrics" 2>/dev/null || true)"
grep -q '^mongodb_up' <<<"$metrics" \
  || { echo "mongodb_up not found on /metrics"; docker logs "$NAME"; exit 1; }

# must run as the nonroot exporter user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot uid: $user, read-only rootfs, /metrics 200)"
