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

# We ship a fix for pip's vendored msgpack (see melange.yaml). Wolfi's pyc files
# are hash-based in UNCHECKED mode, so the interpreter runs the bytecode without
# ever looking at the .py next to it -- meaning a correct-looking source tree can
# still EXECUTE the old vulnerable code, and every CVE scanner will call it clean.
# The only honest check is what the interpreter actually imports at runtime.
echo "== vendored msgpack really executes the fixed version =="
vend="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" \
  -c 'from pip._vendor import msgpack; print(msgpack.__version__)')"
[ "$vend" = "1.2.1" ] || { echo "vendored msgpack is '$vend', expected 1.2.1 (stale bytecode?)"; exit 1; }

echo "== pip's msgpack code path still round-trips =="
docker run --rm --read-only --tmpfs /tmp "$IMAGE" -c '
from pip._vendor import msgpack
from pip._vendor.cachecontrol.serialize import Serializer
payload = {"a": [1, 2, 3], "b": "x"}
assert msgpack.unpackb(msgpack.packb(payload)) == payload
print("msgpack round-trip ok")'

echo "== venv seeds from the rewritten wheel and gets the fixed msgpack =="
seeded="$(docker run --rm --read-only --tmpfs /tmp:rw,exec "$IMAGE" -c '
import venv, subprocess
venv.create("/tmp/v", with_pip=True)
print(subprocess.run(["/tmp/v/bin/python", "-c",
    "from pip._vendor import msgpack; print(msgpack.__version__)"],
    capture_output=True, text=True).stdout.strip())')"
[ "$seeded" = "1.2.1" ] || { echo "venv-seeded msgpack is '$seeded', expected 1.2.1"; exit 1; }

echo "smoke test passed (Python $MINOR, nonroot uid: $uid, read-only rootfs, vendored msgpack $vend)"
