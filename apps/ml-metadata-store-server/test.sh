#!/usr/bin/env bash
# Smoke test for a built ml-metadata-store-server image. Usage: test.sh <image-ref> [version]
# Checks the user, that the server lists the flags Kubeflow Pipelines passes, and that it
# starts serving gRPC (no connection config: an in-memory store).
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got $user"; exit 1; }
out="$(docker run --rm "$IMAGE" --helpfull 2>&1 || true)"
for f in grpc_port mysql_config_host enable_database_upgrade; do
  grep -q -- "-$f" <<<"$out" || { echo "--helpfull lacks -$f"; exit 1; }
done
NAME="mlmd-smoke-$$"
docker run -d --name "$NAME" "$IMAGE" --grpc_port=8080 >/dev/null
trap 'docker rm -f "$NAME" >/dev/null 2>&1 || true' EXIT
for _ in $(seq 1 20); do grep -q "Server listening" <<<"$(docker logs "$NAME" 2>&1)" && break; sleep 1; done
grep -q "Server listening" <<<"$(docker logs "$NAME" 2>&1)" || { docker logs "$NAME" 2>&1 | tail -20; echo "server did not start"; exit 1; }
echo "smoke test passed"
