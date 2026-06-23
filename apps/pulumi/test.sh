#!/usr/bin/env bash
# Smoke test for a built Pulumi image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

echo "pulumi version:"
out="$(docker run --rm "$IMAGE" version 2>&1)"
echo "$out"
# version must be stamped (built from the tag, not the default "v0.0.0"/dev).
# `pulumi version` prints a bare semver like "v3.247.0".
echo "$out" | grep -qiE '^v[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "version not stamped"; exit 1; }

# a real subcommand resolves (proves the binary is functional, not just version)
docker run --rm "$IMAGE" help >/dev/null 2>&1 \
  || { echo "pulumi help failed"; exit 1; }

# must run as the nonroot pulumi user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
