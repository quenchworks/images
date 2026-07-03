#!/usr/bin/env bash
# Smoke test for a built Argo Workflows image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

# The default entrypoint is /usr/bin/workflow-controller, so exercising the
# `argo` binary needs an explicit --entrypoint override.
echo "argo version:"
out="$(docker run --rm --entrypoint /usr/bin/argo "$IMAGE" version 2>&1)"
echo "$out"
# version must be stamped from the tag (not the default v0.0.0).
echo "$out" | grep -qiE 'argo:[[:space:]]*v?[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "argo version not stamped"; exit 1; }

echo "workflow-controller version:"
wc_out="$(docker run --rm "$IMAGE" version 2>&1)"
echo "$wc_out"
echo "$wc_out" | grep -qiE 'v?[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "workflow-controller version not stamped"; exit 1; }

# must run as the nonroot argo user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
