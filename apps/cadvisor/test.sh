#!/usr/bin/env bash
# Smoke test for a built cAdvisor image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

echo "cadvisor version:"
out="$(docker run --rm "$IMAGE" --version 2>&1)"
echo "$out"
# version must be stamped (built from the tag, not the default empty/"unknown").
# cAdvisor prints: "cAdvisor version vX.Y.Z (<revision>)"
echo "$out" | grep -qiE 'cAdvisor version[[:space:]]+v[0-9]+\.[0-9]+' \
  || { echo "version not stamped"; exit 1; }

# must run as the nonroot cadvisor user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
