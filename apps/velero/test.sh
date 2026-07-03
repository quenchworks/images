#!/usr/bin/env bash
# Smoke test for a built Velero image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

echo "velero version (client only):"
out="$(docker run --rm "$IMAGE" version --client-only 2>&1)"
echo "$out"
# version must be stamped (built from the tag, not the default empty/"<no value>")
echo "$out" | grep -qiE 'Version:[[:space:]]*v?[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "version not stamped"; exit 1; }

# `velero --help` must run (server+CLI binary is functional)
docker run --rm "$IMAGE" --help >/dev/null 2>&1 \
  || { echo "velero --help failed"; exit 1; }

# must run as the nonroot velero user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
