#!/usr/bin/env bash
# Smoke test for a built redis image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-redis-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE"
docker run -d --name "$NAME" --read-only --tmpfs /tmp "$IMAGE" >/dev/null

# wait for the server to accept connections
for i in $(seq 1 30); do
  if docker exec "$NAME" redis-cli ping 2>/dev/null | grep -q PONG; then
    break
  fi
  [ "$i" = 30 ] && { echo "redis did not become ready"; docker logs "$NAME"; exit 1; }
  sleep 1
done

echo "PING ok; checking SET/GET and identity"
docker exec "$NAME" redis-cli set qw hello >/dev/null
[ "$(docker exec "$NAME" redis-cli get qw)" = "hello" ] || { echo "SET/GET failed"; exit 1; }

# must run as the nonroot redis user (uid 1001)
uid="$(docker exec "$NAME" id -u)"
[ "$uid" = "1001" ] || { echo "expected uid 1001, got $uid"; exit 1; }

echo "smoke test passed"
