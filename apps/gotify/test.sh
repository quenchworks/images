#!/usr/bin/env bash
# Smoke test for a built Gotify server image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-gotify-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE"
# Server binds 8080 (GOTIFY_SERVER_PORT baked into the image); data goes to a
# tmpfs so no host volume is needed for the smoke test.
docker run -d --name "$NAME" \
  --tmpfs /app/data:uid=1001,gid=1001 \
  -p 127.0.0.1:8080:8080 "$IMAGE" >/dev/null

# /health returns {"health":"green",...} once the DB is up and the server listens.
for i in $(seq 1 30); do
  if curl -fsS http://127.0.0.1:8080/health 2>/dev/null | grep -qi 'green'; then
    break
  fi
  [ "$i" = 30 ] && { echo "gotify did not become healthy"; docker logs "$NAME"; exit 1; }
  sleep 1
done
echo "healthy"

# /version must report the stamped release (not the default "unknown").
ver="$(curl -fsS http://127.0.0.1:8080/version 2>/dev/null || true)"
echo "version endpoint: $ver"
echo "$ver" | grep -qE '"version":"[0-9]+\.[0-9]+\.[0-9]+"' \
  || { echo "version not stamped"; docker logs "$NAME"; exit 1; }

# The embedded UI must be served (go:embed build/*).
curl -fsS http://127.0.0.1:8080/ 2>/dev/null | grep -qi '<!doctype html' \
  || { echo "embedded UI not served"; docker logs "$NAME"; exit 1; }
echo "embedded UI served"

# must run as the nonroot gotify user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
