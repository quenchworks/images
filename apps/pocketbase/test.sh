#!/usr/bin/env bash
# Smoke test for a built PocketBase image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

echo "pocketbase --version:"
# The entrypoint is `pocketbase serve ...`; override args to run --version.
out="$(docker run --rm --entrypoint /usr/bin/pocketbase "$IMAGE" --version 2>&1)"
echo "$out"
# version must be stamped (built from the tag, not the default "(untracked)")
echo "$out" | grep -qiE 'version[[:space:]]+[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "version not stamped"; exit 1; }

# must run as the nonroot pocketbase user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
