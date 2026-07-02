#!/usr/bin/env bash
# Smoke test for a built LiveKit image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

echo "livekit-server --version:"
out="$(docker run --rm "$IMAGE" --version 2>&1)"
echo "$out"
# version must be stamped (built from the tag, reports a real X.Y.Z release)
echo "$out" | grep -qiE '[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "version not reported"; exit 1; }

# must run as the nonroot livekit user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
