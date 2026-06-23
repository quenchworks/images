#!/usr/bin/env bash
# Smoke test for a built Buildkite Agent image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

echo "buildkite-agent --version:"
out="$(docker run --rm "$IMAGE" --version 2>&1)"
echo "$out"
# version must be the real release, embedded from version/VERSION (e.g.
# "buildkite-agent version 3.128.0+quench.<commit>"), not 0.0.0 / unknown.
echo "$out" | grep -qiE 'buildkite-agent version[[:space:]]+[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "version not stamped"; exit 1; }

# a real subcommand resolves (proves the binary is functional, not just --version)
docker run --rm "$IMAGE" start --help >/dev/null 2>&1 \
  || { echo "buildkite-agent start --help failed"; exit 1; }

# must run as the nonroot buildkite-agent user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
