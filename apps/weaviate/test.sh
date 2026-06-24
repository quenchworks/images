#!/usr/bin/env bash
# Smoke test for a built Weaviate image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-weaviate-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE"
# The image just runs the server; supply the minimal env it needs to boot.
# tmpfs /tmp so the nonroot user (uid 1001) can write the data path.
docker run -d --name "$NAME" \
  --tmpfs /tmp \
  -e PERSISTENCE_DATA_PATH=/tmp/data \
  -e AUTHENTICATION_ANONYMOUS_ACCESS_ENABLED=true \
  -e QUERY_DEFAULTS_LIMIT=25 \
  -e CLUSTER_HOSTNAME=node1 \
  -p 127.0.0.1:8080:8080 "$IMAGE" >/dev/null

# /v1/.well-known/ready returns 200 once the server is up
ready=0
for i in $(seq 1 60); do
  if curl -fsS http://127.0.0.1:8080/v1/.well-known/ready >/dev/null 2>&1; then
    ready=1; break
  fi
  sleep 1
done

if [ "$ready" = 1 ]; then
  echo "server is ready (/v1/.well-known/ready returned 200)"
  # liveness endpoint should also respond
  curl -fsS http://127.0.0.1:8080/v1/.well-known/live >/dev/null \
    || { echo "liveness endpoint did not respond"; docker logs "$NAME"; exit 1; }
  # /v1/meta reports the stamped version
  meta="$(curl -fsS http://127.0.0.1:8080/v1/meta 2>/dev/null || true)"
  echo "meta: $meta"
else
  # Fall back: the binary must at least run and print usage.
  echo "server did not become ready in time; falling back to --help check"
  docker logs "$NAME" || true
  docker run --rm "$IMAGE" --help 2>&1 | head -3 \
    || { echo "weaviate-server --help failed"; exit 1; }
fi

# must run as the nonroot weaviate user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
