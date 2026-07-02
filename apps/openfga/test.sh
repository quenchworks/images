#!/usr/bin/env bash
# Smoke test for a built OpenFGA image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-openfga-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

# version must be stamped from the tag (not the default "dev"). The image
# entrypoint is `openfga run`, so override it to reach the version subcommand.
echo "openfga version:"
out="$(docker run --rm --entrypoint /usr/bin/openfga "$IMAGE" version 2>&1)"
echo "$out"
echo "$out" | grep -qiE 'version .v?[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "version not stamped"; exit 1; }

# default command is `openfga run` (in-memory): HTTP 8080, gRPC 8081, playground 3000
echo "starting $IMAGE (in-memory run)"
docker run -d --name "$NAME" -p 127.0.0.1:8080:8080 "$IMAGE" >/dev/null

# the HTTP gateway serves /healthz -> {"status":"SERVING"} once up
for i in $(seq 1 30); do
  if curl -fsS http://127.0.0.1:8080/healthz 2>/dev/null | grep -qi 'SERVING'; then
    break
  fi
  [ "$i" = 30 ] && { echo "openfga did not become healthy"; docker logs "$NAME"; exit 1; }
  sleep 1
done
echo "healthy on :8080/healthz"

# must run as the nonroot openfga user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
