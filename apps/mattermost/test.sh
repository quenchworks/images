#!/usr/bin/env bash
# Smoke test for a built Mattermost (Team Edition) image. Usage: test.sh <image-ref> [expected-version]
# The runtime image is distroless (no shell / coreutils), so process-level checks go
# through the mattermost binary itself and the image config. Presence of the binary +
# client/ webapp tree is asserted at melange build time (melange.yaml test pipeline);
# a full boot needs an external PostgreSQL and is exercised by the chart's kind-install
# gate, not here. For the IMAGE we confirm: the image runs as nonroot uid 1001, and
# `mattermost version` (needs no DB/config) prints the expected Team-Edition build.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
EXPECT_VER="${2:-}"

echo "checking the image runs as nonroot uid 1001"
USER_CFG="$(docker inspect --format '{{.Config.User}}' "$IMAGE")"
echo "  Config.User: $USER_CFG"
[ "$USER_CFG" = "1001" ] || { echo "FAIL: image user is not 1001"; exit 1; }

echo "checking work-dir is the install dir"
WD="$(docker inspect --format '{{.Config.WorkingDir}}' "$IMAGE")"
echo "  WorkingDir: $WD"
[ "$WD" = "/opt/mattermost" ] || { echo "FAIL: work-dir is not /opt/mattermost"; exit 1; }

echo "mattermost version (no DB required), run read-only as the image user:"
VER_OUT="$(docker run --rm --read-only --entrypoint mattermost "$IMAGE" version)"
echo "$VER_OUT" | sed 's/^/  /'
echo "$VER_OUT" | grep -q '^Version: ' || { echo "FAIL: no Version line"; exit 1; }
# Team Edition must NOT be enterprise-ready
echo "$VER_OUT" | grep -q 'Build Enterprise Ready: false' \
  || { echo "FAIL: expected Team Edition (Build Enterprise Ready: false)"; exit 1; }

if [ -n "$EXPECT_VER" ]; then
  echo "$VER_OUT" | grep -q "^Version: ${EXPECT_VER}$" \
    || { echo "FAIL: expected Version: ${EXPECT_VER}"; exit 1; }
  echo "  version matches ${EXPECT_VER}"
fi

echo "PASS: mattermost smoke test green"
