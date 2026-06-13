#!/usr/bin/env bash
# Smoke test for a built Harbor registry image. Usage: test.sh <image-ref>
# Runs with a read-only rootfs (the posture the chart ships), with writable
# /storage and /tmp tmpfs mounts, serving the shipped minimal /etc/registry/
# config.yml (filesystem storage /storage, HTTP :5000, debug/metrics :5001).
# Asserts:
#   - `registry --version` prints the pinned v2.8.3-patch-redis string
#   - GET /v2/  -> 200 (registry API v2 base, the canonical health probe)
#   - GET /     -> 200 (root)
#   - GET :5001/metrics -> 200 (debug/metrics listener up)
#   - the container runs as nonroot uid 1001
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-harbor-registry-smoke-$$"
BASE="http://127.0.0.1:5000"
DEBUG="http://127.0.0.1:5001"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "checking registry --version"
# The image entrypoint is `registry serve /etc/registry/config.yml`; --version
# is a root-command flag, so override the entrypoint to invoke it directly.
ver="$(docker run --rm --entrypoint /usr/bin/registry "$IMAGE" --version 2>&1 || true)"
echo "$ver"
echo "$ver" | grep -q "v2.8.3-patch-redis" || {
  echo "registry --version did not report v2.8.3-patch-redis"; exit 1; }
echo "version OK (v2.8.3-patch-redis)"

echo "starting $IMAGE (read-only rootfs; writable /storage + /tmp tmpfs)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /tmp \
  --tmpfs /storage:uid=1001,gid=1001 \
  -p 127.0.0.1:5000:5000 \
  -p 127.0.0.1:5001:5001 \
  "$IMAGE" >/dev/null

# wait for /v2/ to report 200
for i in $(seq 1 30); do
  code="$(curl -fsS -o /dev/null -w '%{http_code}' "$BASE/v2/" 2>/dev/null || true)"
  if [ "$code" = "200" ]; then
    break
  fi
  [ "$i" = 30 ] && { echo "registry /v2/ did not become healthy"; docker logs "$NAME"; exit 1; }
  sleep 1
done
echo "/v2/ -> 200"

code="$(curl -fsS -o /dev/null -w '%{http_code}' "$BASE/" 2>/dev/null || true)"
[ "$code" = "200" ] || { echo "/ returned $code, expected 200"; docker logs "$NAME"; exit 1; }
echo "/ -> 200"

code="$(curl -fsS -o /dev/null -w '%{http_code}' "$DEBUG/metrics" 2>/dev/null || true)"
[ "$code" = "200" ] || { echo ":5001/metrics returned $code, expected 200"; docker logs "$NAME"; exit 1; }
echo ":5001/metrics -> 200"

# must run as the nonroot registry user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user, read-only rootfs, /v2/ + / + metrics OK)"
