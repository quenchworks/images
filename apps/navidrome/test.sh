#!/usr/bin/env bash
# Smoke test for a built Navidrome image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

echo "navidrome version:"
out="$(docker run --rm "$IMAGE" --version 2>&1)"
echo "$out"
# version must be stamped (built from the tag, not the fallback "dev")
echo "$out" | grep -qiE '[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "version not stamped (got: $out)"; exit 1; }
echo "$out" | grep -qi 'dev' \
  && { echo "version reports 'dev' -- gitSha/gitTag not stamped"; exit 1; } || true

# must run as the nonroot navidrome user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
