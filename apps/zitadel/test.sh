#!/usr/bin/env bash
# Smoke test for a built ZITADEL image. Usage: test.sh <image-ref> [expected-version]
# The runtime image is distroless (no shell / coreutils), so process-level checks go
# through the zitadel binary itself and the image config. A full `start` needs an
# external PostgreSQL + a 32-char master key and is exercised by the chart's
# kind-install gate, not here. For the IMAGE we confirm: the image runs as nonroot
# uid 1001, and `zitadel --version` (needs no DB/config) prints the expected build.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
EXPECT_VER="${2:-}"

echo "checking the image runs as nonroot uid 1001"
USER_CFG="$(docker inspect --format '{{.Config.User}}' "$IMAGE")"
echo "  Config.User: $USER_CFG"
[ "$USER_CFG" = "1001" ] || { echo "FAIL: image user is not 1001"; exit 1; }

echo "checking the entrypoint is /usr/bin/zitadel"
EP="$(docker inspect --format '{{json .Config.Entrypoint}}' "$IMAGE")"
echo "  Entrypoint: $EP"
echo "$EP" | grep -q '/usr/bin/zitadel' || { echo "FAIL: entrypoint is not /usr/bin/zitadel"; exit 1; }

echo "zitadel --version (no DB required), run read-only as the image user:"
VER_OUT="$(docker run --rm --read-only "$IMAGE" --version)"
echo "$VER_OUT" | sed 's/^/  /'
echo "$VER_OUT" | grep -q '^zitadel version ' || { echo "FAIL: no version line"; exit 1; }

if [ -n "$EXPECT_VER" ]; then
  # The binary prints `zitadel version 4.17.1` -- NO leading v. This assertion used to
  # require one, so it failed on every correct build. It never showed up because the app
  # was BLOCKED=1, so CI skipped it and this test had not run since the flag went on:
  # blocking an app silences the check that would have caught this. Accept either form,
  # since upstream has emitted both across releases. Capture then grep -- `echo | grep -q`
  # under pipefail fails WHEN THE PATTERN MATCHES (grep exits, echo takes SIGPIPE).
  grep -qE "^zitadel version v?${EXPECT_VER}$" <<<"$VER_OUT" \
    || { echo "FAIL: expected zitadel version ${EXPECT_VER} (with or without a v), got:"; echo "$VER_OUT"; exit 1; }
  echo "  version matches ${EXPECT_VER}"
fi

echo "PASS: zitadel smoke test green"
