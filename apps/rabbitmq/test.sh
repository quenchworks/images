#!/usr/bin/env bash
# Smoke test for a built rabbitmq image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-rabbitmq-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE"
docker run -d --name "$NAME" "$IMAGE" >/dev/null

# the broker takes a bit to boot. ping comes up at the distribution layer before
# the app finishes, so gate on check_running, which only succeeds once the node is
# fully booted (listeners up, plugins started).
for i in $(seq 1 60); do
  if docker exec "$NAME" rabbitmq-diagnostics -q check_running >/dev/null 2>&1; then
    echo "node fully booted after ~$((i*2))s"; break
  fi
  [ "$i" = 60 ] && { echo "rabbitmq did not become ready"; docker logs "$NAME" | tail -30; exit 1; }
  sleep 2
done

# must run as the nonroot rabbitmq user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "version:"; docker exec "$NAME" rabbitmqctl --version
echo "smoke test passed (nonroot user: $user)"
