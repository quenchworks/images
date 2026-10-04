#!/usr/bin/env bash
# Smoke test for a built longhorn-manager image. Usage: test.sh <image-ref> [version]
# The daemon needs a privileged pod with the host's /proc and iscsiadm, so outside a
# cluster the test runs it with every required flag and expects it to stop at the
# host environment check (after flag parsing, before any API call). The chart gate
# under upstream's Longhorn chart covers the rest.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "0" ] || [ -z "$user" ] || { echo "expected root (upstream's chart sets no user), got '$user'"; exit 1; }

ver="$(docker run --rm --entrypoint longhorn-manager "$IMAGE" --version)"
echo "$ver"
[ -z "$WANT" ] || grep -q "v$WANT" <<<"$ver" || { echo "expected v$WANT"; exit 1; }

docker run --rm --entrypoint bash "$IMAGE" -c \
  'for t in launch-manager nsmounter nsenter mount umount mkfs.ext4 mkfs.xfs mount.nfs; do command -v $t >/dev/null || { echo "missing $t"; exit 1; }; done'

img=example.invalid/x:1
out="$(docker run --rm -e NODE_NAME=smoke --entrypoint longhorn-manager "$IMAGE" -d daemon \
  --engine-image $img --instance-manager-image $img --share-manager-image $img \
  --backing-image-manager-image $img --support-bundle-manager-image $img \
  --manager-image $img --service-account longhorn 2>&1 || true)"
grep -q "failed to check environment" <<<"$out" || { echo "daemon did not reach the environment check:"; tail -20 <<<"$out"; exit 1; }

echo "smoke test passed (longhorn-manager ${WANT:-?}: version, host tools, daemon start path; user ${user:-0})"
