#!/usr/bin/env bash
# Smoke test for a built cephcsi image. Usage: test.sh <image-ref> [version]
# Checks the driver version and the tools the RBD and CephFS node plugins exec. The CSI
# paths themselves are exercised by the rook-ceph chart gate (they need a Ceph cluster).
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
out="$(docker run --rm "$IMAGE" --version)"
echo "$out"
ver="$(sed -n 's/^Cephcsi Version: v\([0-9.]*\).*/\1/p' <<<"$out")"
[ -z "$WANT" ] || [ "$ver" = "$WANT" ] || { echo "expected cephcsi $WANT, got '$ver'"; exit 1; }
docker run --rm --entrypoint /bin/bash "$IMAGE" -c '
set -e
for b in rbd ceph ceph-fuse mount umount blkid blockdev fsck findmnt lsblk mkfs.ext4 e2fsck resize2fs mkfs.xfs xfs_growfs cryptsetup modprobe mount.nfs; do command -v $b >/dev/null || { echo "missing $b"; exit 1; }; done
echo tools present'
echo "smoke test passed"
