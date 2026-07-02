#!/usr/bin/env bash
# Smoke test for a built zot (OCI-native registry) image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-zot-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE"
docker run -d --name "$NAME" -p 127.0.0.1:5000:5000 "$IMAGE" >/dev/null

# /v2/ is the OCI distribution API base; zot returns 200 once the server is ready
for i in $(seq 1 30); do
  code="$(curl -fsS -o /dev/null -w '%{http_code}' http://127.0.0.1:5000/v2/ 2>/dev/null || true)"
  if [ "$code" = "200" ]; then break; fi
  [ "$i" = 30 ] && { echo "zot /v2/ never returned 200 (last: $code)"; docker logs "$NAME"; exit 1; }
  sleep 1
done
echo "/v2/ returned 200"

# version must be stamped (built from the tag, not an empty default). `zot
# --version` logs a JSON line whose commit field carries the v-prefixed release.
echo "zot version:"
out="$(docker exec "$NAME" /usr/bin/zot --version 2>&1)"
echo "$out"
echo "$out" | grep -qiE 'commit[^0-9]*v?[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "version not stamped"; exit 1; }

# must run as the nonroot zot user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
