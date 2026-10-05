#!/usr/bin/env bash
# Smoke test for a built trust-manager image. Usage: test.sh <image-ref> [version]
# trust-manager only works inside a cluster (the chart's kind gate syncs a real Bundle);
# here the binary must run its CLI as nonroot.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
out="$(docker run --rm "$IMAGE" --help 2>&1)" || { echo "--help failed"; echo "$out"; exit 1; }
grep -q 'trust-manager' <<<"$out" || { echo "no trust-manager in --help"; echo "$out"; exit 1; }
grep -q -- '--trust-namespace' <<<"$out" || { echo "no --trust-namespace flag"; echo "$out"; exit 1; }
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed (CLI responds, nonroot user: $user)"
