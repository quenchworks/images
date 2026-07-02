#!/usr/bin/env bash
# Smoke test for a built Centrifugo image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

echo "centrifugo version:"
# prints "Centrifugo v<X.Y.Z> (Go version: ...)". Must be stamped (built from the
# tag, not the default "0.0.0").
out="$(docker run --rm "$IMAGE" version 2>&1)"
echo "$out"
echo "$out" | grep -qiE 'v[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "version not stamped"; exit 1; }
echo "$out" | grep -q 'v0\.0\.0' && { echo "version is default 0.0.0 (not stamped)"; exit 1; }

# must run as the nonroot centrifugo user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

# --- server smoke: boot with the health endpoint enabled and curl /health -----
# Config is entirely via CENTRIFUGO_* env (no writable state) so this exercises
# the read-only-rootfs path too. Centrifugo binds 0.0.0.0:8000 by default.
name="centrifugo-smoke-$$"
port=18000
echo "starting server (health endpoint on :$port) ..."
docker run -d --rm --name "$name" \
  --read-only \
  -e CENTRIFUGO_HEALTH_ENABLED=true \
  -e CENTRIFUGO_CLIENT_ALLOWED_ORIGINS='*' \
  -p "127.0.0.1:${port}:8000" "$IMAGE" >/dev/null

cleanup() { docker rm -f "$name" >/dev/null 2>&1 || true; }
trap cleanup EXIT

ok=0
for i in $(seq 1 20); do
  if curl -fsS "http://127.0.0.1:${port}/health" >/dev/null 2>&1; then ok=1; break; fi
  sleep 0.5
done
if [ "$ok" != 1 ]; then
  echo "health endpoint did not come up; container logs:"
  docker logs "$name" 2>&1 | tail -30
  exit 1
fi
echo "GET /health -> $(curl -fsS "http://127.0.0.1:${port}/health")"

echo "smoke test passed (nonroot user: $user, health OK on read-only rootfs)"
