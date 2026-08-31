#!/usr/bin/env bash
# Smoke test for a built mysqld_exporter image. Usage: test.sh <image-ref> [version]
# Runs read-only rootfs, waits for /metrics on 9104, asserts the build_info metric
# (with the expected stamped version when $2 is given). Confirms the container runs
# as nonroot uid 1001. Works without a MySQL backend: the exporter serves /metrics
# regardless and just reports mysql_up 0. A username is required because the config
# handler refuses to start with no user in the [client] section.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
EXPECT="${2:-}"
NAME="quench-mysqld-exporter-smoke-$$"
BASE="http://127.0.0.1:9104"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "version check:"
ver="$(docker run --rm "$IMAGE" --version 2>&1)"
echo "$ver"
if [ -n "$EXPECT" ]; then
  grep -q "$EXPECT" <<<"$ver" || { echo "expected version $EXPECT in '--version' output"; exit 1; }
fi

echo "starting $IMAGE (read-only rootfs)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /tmp:rw,mode=1777 \
  -e MYSQLD_EXPORTER_PASSWORD=smoke \
  -p 127.0.0.1:9104:9104 \
  "$IMAGE" --mysqld.username=exporter >/dev/null

# wait for /metrics to come up
for i in $(seq 1 30); do
  if curl -fsS "$BASE/metrics" >/dev/null 2>&1; then break; fi
  [ "$i" = 30 ] && { echo "mysqld_exporter did not serve /metrics"; docker logs "$NAME"; exit 1; }
  sleep 1
done

echo "checking /metrics exposes mysqld_exporter_build_info"
# capture fully first: a streaming `grep -q` closes the pipe early and, under
# `set -o pipefail`, SIGPIPEs curl and makes a real match look like a failure.
metrics="$(curl -fsS "$BASE/metrics" 2>/dev/null || true)"
grep -q 'mysqld_exporter_build_info' <<<"$metrics" \
  || { echo "mysqld_exporter_build_info not found on /metrics"; docker logs "$NAME"; exit 1; }
if [ -n "$EXPECT" ]; then
  grep -q "mysqld_exporter_build_info{[^}]*version=\"$EXPECT\"" <<<"$metrics" \
    || { echo "build_info version label != $EXPECT"; docker logs "$NAME"; exit 1; }
fi

# must run as the nonroot exporter user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot uid: $user, read-only rootfs, /metrics 200)"
