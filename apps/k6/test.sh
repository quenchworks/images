#!/usr/bin/env bash
# Smoke test for a built k6 image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

echo "k6 version:"
out="$(docker run --rm "$IMAGE" version 2>&1)"
echo "$out"
# version must be stamped from the tag (k6 vX.Y.Z ...), not empty/dev-only
echo "$out" | grep -qiE 'k6[[:space:]]+v?[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "version not reported"; exit 1; }

# k6 must actually run a script (read-only rootfs + nonroot). Feed a trivial
# script on stdin so nothing needs to be written to the container filesystem.
echo "running a trivial script from stdin:"
script='export default function () {};'
echo "$script" | docker run --rm --read-only -i "$IMAGE" run --quiet --vus 1 --iterations 1 - \
  || { echo "k6 run failed under read-only rootfs"; exit 1; }

# must run as the nonroot k6 user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
