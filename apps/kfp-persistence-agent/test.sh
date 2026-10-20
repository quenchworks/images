#!/usr/bin/env bash
# Smoke test for a built kfp-persistence-agent image. Usage: test.sh <image-ref> [version]
# Checks the user and that the binary starts and parses its flags. The component itself
# needs a cluster, MySQL and the API server: the kubeflow-pipelines chart gate runs it.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
user="$(docker inspect "$IMAGE" --format "{{.Config.User}}")"
[ "$user" = "1001" ] || { echo "expected user 1001, got $user"; exit 1; }
out="$(docker run --rm --entrypoint /usr/bin/persistence_agent "$IMAGE" --help 2>&1 || true)"
grep -q -- "-ttlSecondsAfterWorkflowFinish" <<<"$out" || { echo "/usr/bin/persistence_agent --help lacks -ttlSecondsAfterWorkflowFinish:"; echo "$out" | head -20; exit 1; }

echo "smoke test passed"
