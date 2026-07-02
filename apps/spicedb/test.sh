#!/usr/bin/env bash
# Smoke test for a built SpiceDB image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-spicedb-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "spicedb version:"
out="$(docker run --rm "$IMAGE" version 2>&1)"
echo "$out"
# version must be stamped (built from the tag, not the default "(devel)"/empty)
echo "$out" | grep -qiE 'v?[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "version not stamped"; exit 1; }

# must run as the nonroot spicedb user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

# bring the server up with the in-memory datastore (no external DB needed) and
# confirm it starts serving the gRPC API on 50051.
echo "starting serve-testing"
docker run -d --name "$NAME" -p 127.0.0.1:50051:50051 "$IMAGE" \
  serve-testing --grpc-addr :50051 --readonly-grpc-enabled=false --http-enabled=false >/dev/null

up=0
for i in $(seq 1 30); do
  if (exec 3<>/dev/tcp/127.0.0.1/50051) 2>/dev/null; then up=1; exec 3>&- 3<&-; break; fi
  if ! docker ps -q --no-trunc | grep -q "$(docker inspect --format '{{.Id}}' "$NAME")"; then
    echo "container exited early"; docker logs "$NAME"; exit 1
  fi
  sleep 1
done
[ "$up" = 1 ] || { echo "gRPC port 50051 never became reachable"; docker logs "$NAME"; exit 1; }

echo "gRPC API reachable on 50051"
echo "smoke test passed (nonroot user: $user)"
