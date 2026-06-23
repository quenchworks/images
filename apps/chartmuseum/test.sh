#!/usr/bin/env bash
# Smoke test for a built ChartMuseum image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

echo "chartmuseum version:"
out="$(docker run --rm "$IMAGE" --version 2>&1)"
echo "$out"
# version must be stamped (built from the tag, not an empty/default version).
# upstream prints e.g. "ChartMuseum version 0.16.5 (build )"
echo "$out" | grep -qiE 'ChartMuseum version[[:space:]]+[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "version not stamped"; exit 1; }

# must run as the nonroot chartmuseum user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
