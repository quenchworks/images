#!/usr/bin/env bash
# Smoke test for a built quench-pnpm image. Usage: test.sh <image-ref> <major>
# where <major> is the expected pnpm major version, e.g. 10 or 11.
#
# This is a BUILD/PACKAGE-TOOL image: `pnpm` (run through the node interpreter)
# IS the entrypoint, there is no long-running service. We exercise pnpm + the
# bundled node directly under a READ-ONLY rootfs and confirm the nonroot uid.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> <major>}"
MAJOR="${2:?usage: test.sh <image-ref> <major>}"   # e.g. 10

echo "== pnpm --version matches $MAJOR (default entrypoint) =="
ver="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" --version 2>&1)"
echo "$ver"
case "$ver" in
  "$MAJOR."*) : ;;
  *) echo "expected '$MAJOR.*', got '$ver'"; exit 1 ;;
esac

echo "== node -v works (the runtime backing pnpm) =="
nver="$(docker run --rm --read-only --tmpfs /tmp --entrypoint /usr/bin/node "$IMAGE" -v 2>&1)"
echo "node $nver"
case "$nver" in
  v[0-9]*) : ;;
  *) echo "node -v gave unexpected output: '$nver'"; exit 1 ;;
esac

echo "== pnpm runs a real subcommand (store path on the writable tmpfs) =="
# pnpm probes its store next to the CWD's filesystem, so run from the writable
# /tmp workdir; PNPM_HOME pins the store under /tmp for a read-only rootfs.
sp="$(docker run --rm --read-only --tmpfs /tmp -w /tmp "$IMAGE" store path 2>&1)"
echo "store path: $sp"
case "$sp" in
  /tmp/*) : ;;
  *) echo "expected pnpm store under /tmp, got '$sp'"; exit 1 ;;
esac

echo "== runs as nonroot uid 1001 =="
uid="$(docker run --rm --read-only --tmpfs /tmp --entrypoint /usr/bin/node "$IMAGE" -e 'process.stdout.write(String(process.getuid()))')"
[ "$uid" = "1001" ] || { echo "expected uid 1001, got '$uid'"; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected configured user 1001, got '$user'"; exit 1; }

echo "== read-only rootfs is enforced (write to / must fail) =="
if docker run --rm --read-only --tmpfs /tmp --entrypoint /usr/bin/node "$IMAGE" \
     -e 'require("fs").writeFileSync("/should-fail","x")' >/dev/null 2>&1; then
  echo "rootfs was writable, expected read-only"; exit 1
fi

echo "smoke test passed (pnpm $ver, node $nver, nonroot uid: $uid, read-only rootfs)"
