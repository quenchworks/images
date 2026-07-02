#!/usr/bin/env bash
# Smoke test for a built Ory Hydra image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

echo "hydra version:"
# entrypoint is /usr/bin/hydra, so a trailing "version" overrides the default
# "serve all" cmd and runs `hydra version`.
out="$(docker run --rm "$IMAGE" version 2>&1)"
echo "$out"
# version must be stamped (built from the tag, not the default "master")
echo "$out" | grep -qiE 'Version:[[:space:]]*v?[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "version not stamped"; exit 1; }

# must run as the nonroot hydra user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
