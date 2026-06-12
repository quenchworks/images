#!/usr/bin/env bash
# Smoke test for a built nats-server image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-nats-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE"
# enable JetStream against the data volume to exercise the writable path too
docker run -d --name "$NAME" -p 127.0.0.1:8222:8222 \
  "$IMAGE" -js -sd /data >/dev/null

# wait for the monitoring endpoint to report the server is healthy
for i in $(seq 1 30); do
  if curl -fsS http://127.0.0.1:8222/healthz 2>/dev/null | grep -q '"status":"ok"'; then
    break
  fi
  [ "$i" = 30 ] && { echo "nats did not become healthy"; docker logs "$NAME"; exit 1; }
  sleep 1
done

echo "healthy; checking varz reports JetStream enabled"
curl -fsS http://127.0.0.1:8222/varz | grep -q '"jetstream"' \
  || { echo "monitoring endpoint did not return varz"; exit 1; }

# must run as the nonroot nats user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "version:"; docker exec "$NAME" nats-server --version
echo "smoke test passed (nonroot user: $user)"
