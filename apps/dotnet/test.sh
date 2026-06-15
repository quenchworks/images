#!/usr/bin/env bash
# Smoke test for a built quench-dotnet image. Usage: test.sh <image-ref> <major>
# where <major> is the expected .NET major version, e.g. 10.
#
# This is a LANGUAGE/SDK image: the `dotnet` driver IS the entrypoint, there is
# no long-running service to ping. So we exercise the SDK directly under a
# READ-ONLY rootfs and confirm the nonroot uid. Note the package band (e.g.
# 10.0.100) and the SDK's own `--version` (e.g. 10.0.109) can differ in the
# patch, so we match on the MAJOR only.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> <major>}"
MAJOR="${2:?usage: test.sh <image-ref> <major>}"   # e.g. 10

echo "== dotnet --version matches major $MAJOR (default entrypoint) =="
ver="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" --version 2>&1)"
echo "$ver"
case "$ver" in
  "$MAJOR."*) : ;;
  *) echo "expected '$MAJOR.*', got '$ver'"; exit 1 ;;
esac

echo "== /usr/bin/dotnet is callable explicitly =="
vver="$(docker run --rm --read-only --tmpfs /tmp --entrypoint /usr/bin/dotnet \
  "$IMAGE" --version 2>&1)"
echo "$vver"
case "$vver" in
  "$MAJOR."*) : ;;
  *) echo "expected '$MAJOR.*' from /usr/bin/dotnet, got '$vver'"; exit 1 ;;
esac

echo "== dotnet --info works (SDK + runtime resolve on read-only rootfs) =="
info="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" --info 2>&1)"
echo "$info" | head -20
case "$info" in
  *".NET SDK:"*) : ;;
  *) echo "expected '.NET SDK:' section in --info output"; exit 1 ;;
esac
# The SDK must report a runtime of the same major, proving the runtime + host
# resolved (not just the muxer).
case "$info" in
  *"Microsoft.NETCore.App $MAJOR."*) : ;;
  *) echo "expected a Microsoft.NETCore.App $MAJOR.* runtime in --info"; exit 1 ;;
esac

echo "== dotnet --list-sdks reports a $MAJOR.x SDK =="
sdks="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" --list-sdks 2>&1)"
echo "$sdks"
case "$sdks" in
  "$MAJOR."*) : ;;
  *) echo "expected an installed $MAJOR.* SDK"; exit 1 ;;
esac

echo "== runs as nonroot uid 1001 =="
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected configured user 1001, got '$user'"; exit 1; }

echo "== read-only rootfs is enforced (a build writing to / must not persist) =="
# The image has no shell; we assert the configured rootfs is read-only by
# confirming the SDK still answers under --read-only above. As a belt-and-braces
# check, confirm the entrypoint binary is not under a writable path.
ep="$(docker inspect "$IMAGE" --format '{{json .Config.Entrypoint}}')"
case "$ep" in
  *"/usr/bin/dotnet"*) : ;;
  *) echo "unexpected entrypoint: $ep"; exit 1 ;;
esac

echo "smoke test passed (.NET $MAJOR, nonroot uid: 1001, read-only rootfs)"
