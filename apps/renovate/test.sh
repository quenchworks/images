#!/usr/bin/env bash
# Smoke test for a built Renovate image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

echo "renovate --version:"
out="$(docker run --rm "$IMAGE" --version 2>&1)"
echo "$out"
# version must print a real semver (proves node runtime + bundle resolve)
echo "$out" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "renovate --version did not print a semver"; exit 1; }

# must run as the nonroot renovate user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
