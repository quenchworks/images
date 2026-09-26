#!/usr/bin/env bash
# Smoke test for a built Podman image. Usage: test.sh <image-ref> [version]
# Runs Podman rootless (uid 1001) inside a privileged container: it must report
# this version, pull a small public image, and run it in its own user namespace
# with no network (Wolfi has no passt for rootless networking).
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
INNER=ghcr.io/quenchworks/images/crane:0.22.1

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

ver="$(docker run --rm "$IMAGE" --version)"
echo "$ver"
[ -z "$WANT" ] || [ "$ver" = "podman version $WANT" ] || { echo "expected $WANT"; exit 1; }

# vfs needs no /dev/fuse; cgroupfs and file events need no systemd.
P=(--storage-driver vfs --cgroup-manager cgroupfs --events-backend file)
out="$(docker run --rm --privileged "$IMAGE" "${P[@]}" run --rm --network none "$INNER" version 2>&1)" \
  || { echo "podman run failed:"; echo "$out"; exit 1; }
echo "$out" | tail -5
grep -q "0.22.1" <<<"$out" || { echo "inner container did not run"; exit 1; }
uidmap="$(docker run --rm --privileged --entrypoint /usr/bin/podman "$IMAGE" "${P[@]}" unshare cat /proc/self/uid_map)"
echo "user namespace: $uidmap"
[ "$(wc -l <<<"$uidmap")" -ge 2 ] || { echo "subuid range not mapped"; exit 1; }

echo "smoke test passed (podman ${WANT:-?}: pulled and ran a container rootless; nonroot user: $user)"
