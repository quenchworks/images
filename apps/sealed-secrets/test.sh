#!/usr/bin/env bash
# Smoke test for a built Sealed Secrets controller image. Usage: test.sh <image-ref>
# The controller only does real work inside a cluster (it needs the API server and the
# SealedSecret CRD; the chart's kind install gate is the runtime test). Here we verify the
# binary executes, reports a stamped version, and runs as nonroot.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref>}"

echo "checking --version reports a stamped release"
ver="$(docker run --rm "$IMAGE" --version 2>&1 | awk '/controller version:/{print $NF}')"
echo "reported version: $ver"
case "$ver" in
  ""|0.0.0|*dev*|*unknown*) echo "version not stamped: '$ver'"; exit 1 ;;
esac

echo "checking the controller CLI responds"
out="$(docker run --rm "$IMAGE" --help 2>&1 || true)"
echo "$out" | grep -q -- "--key-prefix" || { echo "no controller flags in --help"; echo "$out"; exit 1; }
echo "$out" | grep -q -- "--listen-addr" || { echo "no --listen-addr in --help"; echo "$out"; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed (version $ver, nonroot user: $user)"
