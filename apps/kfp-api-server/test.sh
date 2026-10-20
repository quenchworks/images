#!/usr/bin/env bash
# Smoke test for a built kfp-api-server image. Usage: test.sh <image-ref> [version]
# Checks the user and that the binary starts and parses its flags. The component itself
# needs a cluster, MySQL and the API server: the kubeflow-pipelines chart gate runs it.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
user="$(docker inspect "$IMAGE" --format "{{.Config.User}}")"
[ "$user" = "1001" ] || { echo "expected user 1001, got $user"; exit 1; }
out="$(docker run --rm --entrypoint /usr/bin/apiserver "$IMAGE" --help 2>&1 || true)"
grep -q -- "-sampleconfig" <<<"$out" || { echo "/usr/bin/apiserver --help lacks -sampleconfig:"; echo "$out" | head -20; exit 1; }
env="$(docker inspect "$IMAGE" --format "{{json .Config.Env}}")"
[ -z "${2:-}" ] || grep -q "TAG_NAME=$2" <<<"$env" || { echo "TAG_NAME is not $2: $env"; exit 1; }
echo "smoke test passed"
