#!/usr/bin/env bash
# Smoke test for a built Woodpecker CI image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

echo "woodpecker-server --version:"
out="$(docker run --rm --entrypoint /usr/bin/woodpecker-server "$IMAGE" --version 2>&1)"
echo "$out"
# version must be stamped (built from the tag, not the default "dev")
echo "$out" | grep -qE '[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "server version not stamped"; exit 1; }

echo "woodpecker-agent --version:"
docker run --rm --entrypoint /usr/bin/woodpecker-agent "$IMAGE" --version 2>&1 \
  | grep -qE '[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "agent version not stamped"; exit 1; }

echo "woodpecker-cli --version:"
docker run --rm --entrypoint /usr/bin/woodpecker-cli "$IMAGE" --version 2>&1 \
  | grep -qE '[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "cli version not stamped"; exit 1; }

# must run as the nonroot woodpecker user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
