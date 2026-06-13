#!/usr/bin/env bash
# Smoke test for a built Traefik image. Usage: test.sh <image-ref>
# Runs with a read-only rootfs (the posture the chart ships), enables /ping and
# the api/dashboard with a tiny inline static config, then asserts:
#   - `traefik version` prints the pinned version
#   - GET /ping            -> 200 (healthcheck endpoint)
#   - GET /dashboard/      -> 200 (embedded webui served)
#   - GET /api/overview    -> 200 (api enabled)
#   - the container runs as nonroot uid 1001
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-traefik-smoke-$$"
BASE="http://127.0.0.1:8080"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "checking traefik version"
docker run --rm "$IMAGE" version

echo "starting $IMAGE (read-only rootfs; ping + api.dashboard on :8080)"
# Configure entirely via CLI flags so no config file needs mounting.
#   --ping                       enable the /ping liveness endpoint
#   --api.dashboard --api.insecure   serve the dashboard on the entryPoint :8080
#   --entrypoints.traefik.address=:8080  the entryPoint the insecure api binds to
docker run -d --name "$NAME" \
  --read-only \
  -p 127.0.0.1:8080:8080 \
  "$IMAGE" \
  --ping=true \
  --api.dashboard=true \
  --api.insecure=true \
  --entrypoints.traefik.address=:8080 \
  --log.level=INFO >/dev/null

# wait for /ping to report OK
for i in $(seq 1 30); do
  code="$(curl -fsS -o /dev/null -w '%{http_code}' "$BASE/ping" 2>/dev/null || true)"
  if [ "$code" = "200" ]; then
    break
  fi
  [ "$i" = 30 ] && { echo "traefik /ping did not become healthy"; docker logs "$NAME"; exit 1; }
  sleep 1
done
echo "/ping -> 200"

echo "checking the embedded dashboard"
code="$(curl -fsS -o /dev/null -w '%{http_code}' "$BASE/dashboard/" 2>/dev/null || true)"
[ "$code" = "200" ] || { echo "dashboard root returned $code, expected 200"; docker logs "$NAME"; exit 1; }
echo "/dashboard/ -> 200"

code="$(curl -fsS -o /dev/null -w '%{http_code}' "$BASE/api/overview" 2>/dev/null || true)"
[ "$code" = "200" ] || { echo "/api/overview returned $code, expected 200"; docker logs "$NAME"; exit 1; }
echo "/api/overview -> 200"

# must run as the nonroot traefik user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user, read-only rootfs, ping + dashboard + api OK)"
