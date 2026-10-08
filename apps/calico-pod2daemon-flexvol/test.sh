#!/usr/bin/env bash
# Smoke test for a built calico-pod2daemon-flexvol image. Usage: test.sh <image-ref> [version]
# The installer writes a host path, so check the binary and the script are present and the driver answers init.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
out="$(docker run --rm --entrypoint /usr/local/bin/flexvol "$IMAGE" init 2>&1 || true)"
echo "$out"
grep -q '"status"' <<<"$out" || { echo "flexvol init gave no status"; exit 1; }
docker run --rm --entrypoint /bin/bash "$IMAGE" -c 'test -x /usr/local/bin/flexvol.sh && command -v cp mv chmod'
echo "smoke test passed"
