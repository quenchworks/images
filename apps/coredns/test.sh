#!/usr/bin/env bash
# Smoke test for a built CoreDNS image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-coredns-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE"
# default Corefile serves health on 8080 and ready on 8181
docker run -d --name "$NAME" \
  -p 127.0.0.1:8080:8080 -p 127.0.0.1:8181:8181 "$IMAGE" >/dev/null

# health plugin returns 200 "OK" once the server is up
for i in $(seq 1 30); do
  if curl -fsS http://127.0.0.1:8080/health 2>/dev/null | grep -qi 'OK'; then
    break
  fi
  [ "$i" = 30 ] && { echo "coredns did not become healthy"; docker logs "$NAME"; exit 1; }
  sleep 1
done

echo "healthy; checking the ready endpoint"
curl -fsS http://127.0.0.1:8181/ready >/dev/null \
  || { echo "ready endpoint did not respond"; docker logs "$NAME"; exit 1; }

# must run as the nonroot coredns user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "version:"; docker exec "$NAME" coredns -version
echo "smoke test passed (nonroot user: $user)"
