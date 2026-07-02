#!/usr/bin/env bash
# Smoke test for a built MediaMTX image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

echo "mediamtx --version:"
out="$(docker run --rm --entrypoint /usr/bin/mediamtx "$IMAGE" --version 2>&1)"
echo "$out"
# version must be stamped (built from the tag, not the default v0.0.0)
echo "$out" | grep -qiE 'v[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "version not stamped"; exit 1; }
echo "$out" | grep -qi 'v0\.0\.0' && { echo "version fell back to v0.0.0"; exit 1; }

# must run as the nonroot mediamtx user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

# start the server (API enabled via env override) and probe the API
name="mediamtx-smoke-$$"
docker run -d --rm --name "$name" -e MTX_API=yes -p 19997:9997 "$IMAGE" >/dev/null
cleanup() { docker rm -f "$name" >/dev/null 2>&1 || true; }
trap cleanup EXIT

# Any HTTP status (200, or 401 when auth is on by default) proves the API
# listener is up and serving HTTP.
code=000
for _ in $(seq 1 20); do
  code="$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:19997/v3/config/global/get || true)"
  [ "$code" != 000 ] && break
  sleep 0.5
done
[ "$code" != 000 ] || { echo "API on 9997 did not respond"; docker logs "$name" 2>&1 | tail -20; exit 1; }

echo "smoke test passed (nonroot user: $user, API on 9997 responded HTTP $code)"
