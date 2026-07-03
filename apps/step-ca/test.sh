#!/usr/bin/env bash
# Smoke test for a built step-ca image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

echo "step-ca version:"
out="$(docker run --rm --entrypoint /usr/bin/step-ca "$IMAGE" version 2>&1)"
echo "$out"
# version must be stamped (built from the tag, not the default "N/A")
echo "$out" | grep -qiE '[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "version not stamped"; exit 1; }
if echo "$out" | grep -qi 'N/A'; then echo "version is N/A (not stamped)"; exit 1; fi

# must run as the nonroot step user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
