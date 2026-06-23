#!/usr/bin/env bash
# Smoke test for a built Adminer image. Usage: test.sh <image-ref>
# Boots the container (PHP built-in server on 8080), then asserts the Adminer
# login page is served over HTTP. A read-only rootfs is fine; PHP only needs a
# writable /tmp for session/upload scratch, so we mount a tmpfs there.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="adminer-smoke-$$"
PORT=8099

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

# must be the nonroot adminer user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

docker run --rm -d --name "$NAME" \
  --tmpfs /tmp \
  -p "127.0.0.1:${PORT}:8080" \
  "$IMAGE" >/dev/null

# wait for the server to accept connections
ok=""
for i in $(seq 1 30); do
  code="$(curl -fsS -o /tmp/adminer-out.html -w '%{http_code}' "http://127.0.0.1:${PORT}/" 2>/dev/null || true)"
  if [ "$code" = "200" ]; then ok=1; break; fi
  sleep 1
done
[ -n "$ok" ] || { echo "adminer did not serve HTTP 200"; docker logs "$NAME" 2>&1 | tail -20; exit 1; }

# body must be the Adminer login page
grep -qi 'adminer' /tmp/adminer-out.html || { echo "body does not mention Adminer"; exit 1; }
grep -qi 'login'   /tmp/adminer-out.html || { echo "body does not look like the login page"; exit 1; }

echo "smoke test passed (HTTP 200, Adminer login served, nonroot user: $user)"
