#!/usr/bin/env bash
# Smoke test for a built quench-dotnet-runtime image.
# Usage: test.sh <image-ref> <major>   where <major> is e.g. 10.
#
# This is a LANGUAGE RUNTIME image: the `dotnet` driver IS the entrypoint, there
# is no long-running service to ping. So we exercise the runtime directly under
# a READ-ONLY rootfs and confirm the nonroot uid. Crucially this is
# RUNTIME-ONLY: we assert the SDK is ABSENT (no `--list-sdks` entry, no `build`
# verb), which is what distinguishes this image from quench-dotnet.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> <major>}"
MAJOR="${2:?usage: test.sh <image-ref> <major>}"   # e.g. 10

echo "== dotnet --list-runtimes shows a $MAJOR.x runtime (default entrypoint) =="
rts="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" --list-runtimes 2>&1)"
echo "$rts"
case "$rts" in
  *"Microsoft.NETCore.App $MAJOR."*) : ;;
  *) echo "FAIL: expected 'Microsoft.NETCore.App $MAJOR.*' in --list-runtimes"; exit 1 ;;
esac

echo "== /usr/bin/dotnet is callable explicitly =="
vrts="$(docker run --rm --read-only --tmpfs /tmp --entrypoint /usr/bin/dotnet \
  "$IMAGE" --list-runtimes 2>&1)"
case "$vrts" in
  *"Microsoft.NETCore.App $MAJOR."*) : ;;
  *) echo "FAIL: expected runtime $MAJOR.* from /usr/bin/dotnet"; exit 1 ;;
esac

echo "== the SDK is ABSENT (this is a runtime-only image, not the SDK) =="
# A runtime image must NOT report any installed SDK.
sdks="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" --list-sdks 2>&1 || true)"
echo "--list-sdks: [$sdks]"
case "$sdks" in
  *"$MAJOR."*) echo "FAIL: an SDK is installed -- this image is not runtime-only"; exit 1 ;;
  *) : ;;
esac
# The `build` verb belongs to the SDK; it must not resolve here.
if docker run --rm --read-only --tmpfs /tmp "$IMAGE" build --help >/dev/null 2>&1; then
  echo "FAIL: 'dotnet build' resolved -- SDK present"; exit 1
fi
echo "ok: no SDK (no --list-sdks entry, no 'build' verb)"

echo "== dotnet --info reports the shared runtime on a read-only rootfs =="
info="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" --info 2>&1)"
echo "$info" | head -20
case "$info" in
  *"Microsoft.NETCore.App $MAJOR."*) : ;;
  *) echo "FAIL: expected a Microsoft.NETCore.App $MAJOR.* runtime in --info"; exit 1 ;;
esac

echo "== runs as configured nonroot uid 1001 =="
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected configured user 1001, got '$user'"; exit 1; }

echo "== entrypoint is the dotnet driver =="
ep="$(docker inspect "$IMAGE" --format '{{json .Config.Entrypoint}}')"
case "$ep" in
  *"/usr/bin/dotnet"*) : ;;
  *) echo "unexpected entrypoint: $ep"; exit 1 ;;
esac

echo "smoke test passed (.NET runtime $MAJOR, runtime-only, nonroot uid: 1001, read-only rootfs)"
