#!/usr/bin/env bash
# Smoke test for a built OpenTofu image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

echo "tofu version:"
out="$(docker run --rm "$IMAGE" version 2>&1)"
echo "$out"
# version must be the real release (built from the tag, not the default "-dev")
echo "$out" | grep -qiE 'OpenTofu v[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "version not stamped"; exit 1; }
echo "$out" | grep -qi 'dev' \
  && { echo "version still reports -dev"; exit 1; }

# a real subcommand resolves (proves the binary is functional, not just --help)
docker run --rm "$IMAGE" init --help >/dev/null 2>&1 \
  || { echo "tofu init --help failed"; exit 1; }

# must run as the nonroot opentofu user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
