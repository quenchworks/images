#!/usr/bin/env bash
# Smoke test for a built Buildah image. Usage: test.sh <image-ref> [version]
# Runs Buildah rootless (uid 1001) inside a privileged container: it must report
# this version, run rootless, and build an image from a Containerfile on a
# small public base (the build needs the subuid range mapped), then read the
# new label back from its storage.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
BASE=ghcr.io/quenchworks/images/crane:0.22.1
VOL="buildah-smoke-$$"
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
cleanup() { docker volume rm -f "$VOL" >/dev/null 2>&1 || true; rm -rf "$WORK"; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

ver="$(docker run --rm "$IMAGE" --version)"
echo "$ver"
[ -z "$WANT" ] || grep -q "^buildah version $WANT " <<<"$ver" || { echo "expected $WANT"; exit 1; }

printf 'FROM %s\nLABEL quench.smoke=built-by-buildah\n' "$BASE" > "$WORK/Containerfile"
chmod -R a+rX "$WORK"
# A named volume keeps the storage between the build and the inspect; vfs
# needs no /dev/fuse.
docker volume create "$VOL" >/dev/null
rootless="$(docker run --rm --privileged "$IMAGE" --storage-driver vfs info --format '{{.host.rootless}}')"
echo "rootless: $rootless"
[ "$rootless" = true ] || { echo "buildah is not running rootless"; exit 1; }
docker run --rm --privileged -v "$VOL:/home/nonroot/.local/share/containers" -v "$WORK:/ctx:ro" "$IMAGE" \
  --storage-driver vfs build --isolation chroot -t smoke:latest -f /ctx/Containerfile /ctx \
  || { echo "buildah build failed"; exit 1; }
label="$(docker run --rm --privileged -v "$VOL:/home/nonroot/.local/share/containers" "$IMAGE" \
  --storage-driver vfs inspect --type image --format '{{index .OCIv1.Config.Labels "quench.smoke"}}' smoke:latest)"
echo "label: $label"
[ "$label" = built-by-buildah ] || { echo "built image lacks the label"; exit 1; }

echo "smoke test passed (buildah ${WANT:-?}: rootless build from a Containerfile; nonroot user: $user)"
