#!/usr/bin/env bash
# Smoke test for a built vCluster syncer image. Usage: test.sh <image-ref> [version]
# The syncer needs a host cluster and the Kubernetes binaries to start, so this
# checks what the image can prove alone: the stamped version, the start command's
# flag parsing, and the nonroot user. The chart gate starts a real virtual cluster.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

ver="$(docker run --rm --entrypoint /vcluster "$IMAGE" version)"
echo "vcluster: $ver"
case "$ver" in ""|dev|*0.0.0*) echo "version not stamped: '$ver'"; exit 1 ;; esac
if [ -n "$WANT" ] && [ "$ver" != "$WANT" ]; then echo "expected $WANT"; exit 1; fi

docker run --rm --entrypoint /vcluster "$IMAGE" start --help >/dev/null \
  || { echo "vcluster start --help failed"; exit 1; }

echo "smoke test passed ($ver, nonroot user: $user)"
