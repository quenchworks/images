#!/usr/bin/env bash
# Smoke test for a built n8n image. Usage: test.sh <image-ref>
#
# Boots n8n against its default standalone sqlite DB (stored under ~/.n8n). We point HOME at
# a writable tmpfs so no PVC is needed for the smoke test. n8n serves the editor/API on 5678.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-n8n-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE"
# n8n listens on 5678. It writes its sqlite DB + config + encryption key under ~/.n8n
# (HOME=/home/node in the image); back that with a writable tmpfs so a standalone boot works
# without a PVC. N8N_DIAGNOSTICS/telemetry off keeps the smoke test hermetic.
docker run -d --name "$NAME" \
  --tmpfs /home/node:rw,mode=0777,uid=1001,gid=1001 \
  --tmpfs /tmp:rw,mode=1777 \
  -e N8N_DIAGNOSTICS_ENABLED=false \
  -e N8N_VERSION_NOTIFICATIONS_ENABLED=false \
  -p 127.0.0.1:5678:5678 "$IMAGE" >/dev/null

# n8n must answer on 5678 (200, or a 3xx redirect to the setup/login page) once it has
# booted + run migrations. First boot generates the encryption key + migrates sqlite, which
# can take a bit, so we poll generously.
code=""
for i in $(seq 1 90); do
  code="$(curl -fsS -o /dev/null -w '%{http_code}' http://127.0.0.1:5678/ 2>/dev/null || true)"
  case "$code" in
    200|301|302|307) break ;;
  esac
  if [ "$i" = 90 ]; then echo "n8n did not start serving (last code: '$code')"; docker logs "$NAME" | tail -40; exit 1; fi
  sleep 2
done
echo "GET / -> $code"

# The healthz endpoint is the authoritative "server is up" probe.
health="$(curl -fsS http://127.0.0.1:5678/healthz 2>/dev/null || true)"
echo "GET /healthz -> $health"
echo "$health" | grep -qi 'ok' \
  || { echo "healthz did not report ok"; docker logs "$NAME" | tail -40; exit 1; }

# must run as the nonroot node user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
