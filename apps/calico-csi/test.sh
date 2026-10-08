#!/usr/bin/env bash
# Smoke test for a built calico-csi image. Usage: test.sh <image-ref> [version]
# The driver serves a kubelet socket on a node; check it starts and prints its flag usage.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
out="$(docker run --rm --entrypoint /usr/bin/csi-driver "$IMAGE" -help 2>&1 || true)"
echo "$out" | head -5
grep -q -- -endpoint <<<"$out" || { echo "csi-driver did not print its usage"; exit 1; }
echo "smoke test passed"
