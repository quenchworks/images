#!/usr/bin/env bash
# Smoke test for a built Tekton Pipelines image. Usage: test.sh <image-ref>
# The components (controller/webhook/resolvers) are long-running servers that need
# a cluster, so we exercise `nop` -- a bundled Tekton binary that runs and exits 0
# with no external dependencies -- to prove the image and a shipped binary work,
# then assert the packaging (default entrypoint = controller, nonroot uid 1001).
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

echo "running bundled 'nop' binary:"
out="$(docker run --rm --entrypoint /usr/bin/nop "$IMAGE" 2>&1)"
echo "$out"
echo "$out" | grep -qi 'Exiting' || { echo "nop did not run as expected"; exit 1; }

# default entrypoint must be the controller
ep="$(docker inspect "$IMAGE" --format '{{json .Config.Entrypoint}}')"
echo "entrypoint: $ep"
echo "$ep" | grep -q '/usr/bin/controller' || { echo "expected controller entrypoint, got '$ep'"; exit 1; }

# must run as the nonroot tekton user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
