#!/usr/bin/env bash
# Smoke test for a built secrets-store-csi-driver image. Usage: test.sh <image-ref> [version]
# The driver serves a kubelet socket on a node; check its version stamp and that mount is present.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
out="$(docker run --rm "$IMAGE" --version 2>&1 || true)"
echo "secrets-store-csi --version: $out"
[ -z "$WANT" ] || grep -q "v$WANT" <<<"$out" || { echo "expected v$WANT"; exit 1; }
for b in /usr/bin/mount /usr/bin/umount; do
  docker run --rm --entrypoint "$b" "$IMAGE" --version >/dev/null 2>&1 || { echo "$b missing"; exit 1; }
done
echo "smoke test passed"
