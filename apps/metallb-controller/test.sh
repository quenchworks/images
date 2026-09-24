#!/usr/bin/env bash
# Smoke test for a built metallb-controller image. Usage: test.sh <image-ref> <version>
# The controller needs a Kubernetes API server to do real work, so outside a cluster it logs
# its startup line (with the stamped version) and then fails to reach the API. The test
# asserts that line and the nonroot default user; the chart's kind gate does the rest.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> <version>}"
VERSION="${2:?usage: test.sh <image-ref> <version>}"

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

logs="$(timeout 30 docker run --rm "$IMAGE" 2>&1 || true)"
echo "$logs" | head -5
echo "$logs" | grep -q "MetalLB controller starting version ${VERSION}" \
  || { echo "startup line with version ${VERSION} not found"; exit 1; }
echo "$logs" | grep -q '"commit":"[0-9a-f]\{8\}"' \
  || { echo "commit not stamped"; exit 1; }

echo "smoke test passed (nonroot user $user, controller version ${VERSION})"
