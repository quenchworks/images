#!/usr/bin/env bash
# Smoke test for a built quench-python image. Usage: test.sh <image-ref> <minor>
# where <minor> is the expected Python minor version, e.g. 3.13.
#
# This is a LANGUAGE/BASE image: the interpreter IS the entrypoint, there is no
# long-running service to ping. So we exercise the interpreter + pip directly
# under a READ-ONLY rootfs and confirm the nonroot uid.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> <minor>}"
MINOR="${2:?usage: test.sh <image-ref> <minor>}"   # e.g. 3.13

echo "== python3 --version matches $MINOR (default entrypoint) =="
ver="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" --version 2>&1)"
echo "$ver"
case "$ver" in
  "Python $MINOR."*) : ;;
  *) echo "expected 'Python $MINOR.*', got '$ver'"; exit 1 ;;
esac

echo "== /usr/bin/python$MINOR is callable explicitly =="
# Wolfi installs the versioned interpreter at /usr/bin/python<minor> (e.g.
# /usr/bin/python3.13), with python3/python symlinking to it.
vver="$(docker run --rm --read-only --tmpfs /tmp --entrypoint "/usr/bin/python$MINOR" \
  "$IMAGE" --version 2>&1)"
echo "$vver"
case "$vver" in
  "Python $MINOR."*) : ;;
  *) echo "expected 'Python $MINOR.*' from /usr/bin/python$MINOR, got '$vver'"; exit 1 ;;
esac

echo "== a real script runs =="
out="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" -c 'print("quench-" + "python")')"
[ "$out" = "quench-python" ] || { echo "script output wrong: '$out'"; exit 1; }

echo "== stdlib import sanity (ssl uses ca-certificates-bundle) =="
docker run --rm --read-only --tmpfs /tmp "$IMAGE" \
  -c 'import ssl, json, hashlib, sqlite3; print("stdlib ok", ssl.OPENSSL_VERSION)'

echo "== pip works (read-only rootfs, writable /tmp) =="
docker run --rm --read-only --tmpfs /tmp "$IMAGE" -m pip --version

echo "== runs as nonroot uid 1001 =="
uid="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" -c 'import os; print(os.getuid())')"
[ "$uid" = "1001" ] || { echo "expected uid 1001, got '$uid'"; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected configured user 1001, got '$user'"; exit 1; }

echo "== read-only rootfs is enforced (write to / must fail) =="
if docker run --rm --read-only --tmpfs /tmp "$IMAGE" \
     -c 'open("/should-fail","w")' >/dev/null 2>&1; then
  echo "rootfs was writable, expected read-only"; exit 1
fi

echo "smoke test passed (Python $MINOR, nonroot uid: $uid, read-only rootfs)"
