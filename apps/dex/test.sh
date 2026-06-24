#!/usr/bin/env bash
# Smoke test for a built Dex image. Usage: test.sh <image-ref>
# Dex needs a config file to `serve`, so the smoke test exercises the `version`
# subcommand (no config required) and confirms the binary is stamped + nonroot.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

echo "checking $IMAGE reports a stamped version"
# `version` is config-free; output line is "Dex Version: X.Y.Z"
ver="$(docker run --rm --entrypoint /usr/bin/dex "$IMAGE" version | awk -F': ' '/^Dex Version:/{print $2}')"
echo "reported version: $ver"
case "$ver" in
  ""|0.0.0|DEV|*dev*) echo "version not stamped: '$ver'"; exit 1 ;;
esac

# must run as the nonroot dex user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (version $ver, nonroot user: $user)"
