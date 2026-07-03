#!/usr/bin/env bash
# Smoke test for a built Miniflux image. Usage: test.sh <image-ref>
# Miniflux needs a PostgreSQL server to actually serve, so this is a binary-level
# smoke test (like grype): it checks the stamped version and the nonroot user.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

echo "miniflux version:"
out="$(docker run --rm "$IMAGE" -version 2>&1)"
echo "$out"
# version must be the stamped release (X.Y.Z), not the "2.3.x-dev" build-info fallback
echo "$out" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$' \
  || { echo "version not stamped (got '$out')"; exit 1; }

# -info prints the full build information block
echo "build info:"
docker run --rm "$IMAGE" -info 2>&1 || true

# must run as the nonroot miniflux user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
