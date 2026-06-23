#!/usr/bin/env bash
# Smoke test for a built DuckDB image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

echo "duckdb version:"
out="$(docker run --rm "$IMAGE" --version 2>&1)"
echo "$out"
# version must report a real vX.Y.Z (the official binary stamps its release version)
echo "$out" | grep -qiE 'v[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "version not stamped"; exit 1; }

# must run as the nonroot duckdb user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
