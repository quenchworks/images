#!/usr/bin/env bash
# Smoke test for a built Distribution (OCI registry) image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-distribution-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE"
docker run -d --name "$NAME" -p 127.0.0.1:5000:5000 "$IMAGE" >/dev/null

# /v2/ is the registry API base; it returns 200 once the server is ready
for i in $(seq 1 30); do
  code="$(curl -fsS -o /dev/null -w '%{http_code}' http://127.0.0.1:5000/v2/ 2>/dev/null || true)"
  if [ "$code" = "200" ]; then break; fi
  [ "$i" = 30 ] && { echo "registry /v2/ never returned 200 (last: $code)"; docker logs "$NAME"; exit 1; }
  sleep 1
done

# must run as the nonroot registry user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "version:"; docker exec "$NAME" registry --version || true
echo "smoke test passed (nonroot user: $user)"
