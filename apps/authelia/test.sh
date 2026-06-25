#!/usr/bin/env bash
# Smoke test for a built Authelia image. Usage: test.sh <image-ref> [expected-version]
# The runtime image is distroless (no shell / coreutils), so process-level checks go
# through the authelia binary itself and the image config. Authelia needs a config file
# + a storage backend (sqlite/postgres) + optional redis to actually `serve`, so a full
# boot is exercised by the chart's kind-install gate, not here. For the IMAGE we confirm:
# the image runs as nonroot uid 1001, and `authelia --version` (needs no config/DB)
# prints the expected build. Presence of the binary is asserted at melange build time
# (melange.yaml test pipeline).
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
EXPECT_VER="${2:-}"

echo "checking the image runs as nonroot uid 1001"
USER_CFG="$(docker inspect --format '{{.Config.User}}' "$IMAGE")"
echo "  Config.User: $USER_CFG"
[ "$USER_CFG" = "1001" ] || { echo "FAIL: image user is not 1001"; exit 1; }

echo "authelia --version (no config/DB required), run read-only as the image user:"
VER_OUT="$(docker run --rm --read-only "$IMAGE" --version)"
echo "$VER_OUT" | sed 's/^/  /'
echo "$VER_OUT" | grep -q '^authelia version v' || { echo "FAIL: no 'authelia version' line"; exit 1; }

if [ -n "$EXPECT_VER" ]; then
  echo "$VER_OUT" | grep -q "^authelia version v${EXPECT_VER}\$" \
    || { echo "FAIL: expected authelia version v${EXPECT_VER}"; exit 1; }
  echo "  version matches v${EXPECT_VER}"
fi

echo "PASS: authelia smoke test green"
