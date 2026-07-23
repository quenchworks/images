#!/usr/bin/env bash
# Smoke test for a built TigerBeetle image. Usage: test.sh <image-ref>
# Confirms nonroot uid 1001, reports `tigerbeetle version`, and formats a data file
# on a writable tmpfs /data (with a read-only rootfs) to prove the binary runs and
# the format path works. TigerBeetle speaks its own binary protocol (not HTTP), so a
# start/connect roundtrip belongs in the chart's install gate, not here.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

echo "version:"
docker run --rm "$IMAGE" version

echo "uid check:"
UID_OUT="$(docker run --rm --entrypoint /bin/sh "$IMAGE" -c 'id -u')"
echo "  runtime uid: $UID_OUT"
[ "$UID_OUT" = "1001" ] || { echo "FAIL: not running as uid 1001"; exit 1; }

echo "format a data file (read-only rootfs, writable tmpfs /data):"
# TigerBeetle does ALL I/O through io_uring, whose syscalls the default Docker/
# containerd seccomp profile blocks (returns EPERM -> "io_uring is not available").
# io_uring is enabled at the kernel level; only the seccomp filter needs opening, so
# run unconfined. The chart makes the same trade-off via podSecurityContext.
docker run --rm \
  --security-opt seccomp=unconfined \
  --read-only \
  --tmpfs /data:rw,mode=0700,uid=1001,gid=1001 \
  "$IMAGE" \
  format --cluster=0 --replica=0 --replica-count=1 /data/0_0.tigerbeetle

echo "PASS: tigerbeetle smoke test green"
