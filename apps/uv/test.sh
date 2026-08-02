#!/usr/bin/env bash
# Smoke test for a built quench-uv image. Usage: test.sh <image-ref> <pyminor>
# where <pyminor> is the expected underlying Python minor, e.g. 3.13.
#
# This is a BUILD/PACKAGE-TOOL image: the self-contained `uv` binary IS the
# entrypoint, there is no long-running service. We exercise uv + the bundled
# Python directly under a READ-ONLY rootfs and confirm the nonroot uid. uv's
# cache lives on the writable /tmp tmpfs.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> <pyminor>}"
PYMINOR="${2:?usage: test.sh <image-ref> <pyminor>}"   # e.g. 3.13

echo "== uv --version works (default entrypoint) =="
ver="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" --version 2>&1)"
echo "$ver"
case "$ver" in
  "uv "[0-9]*) : ;;
  *) echo "expected 'uv <version>', got '$ver'"; exit 1 ;;
esac

echo "== python3 --version matches $PYMINOR (the interpreter uv targets) =="
pver="$(docker run --rm --read-only --tmpfs /tmp --entrypoint /usr/bin/python3 "$IMAGE" --version 2>&1)"
echo "$pver"
case "$pver" in
  "Python $PYMINOR."*) : ;;
  *) echo "expected 'Python $PYMINOR.*', got '$pver'"; exit 1 ;;
esac

echo "== uv pip list runs against the system Python (writable cache on /tmp) =="
# The image deliberately ships NO pip package (its vendored tree is unfixable --
# see apko.yaml), so the interpreter has NO site-packages and an EMPTY listing is
# the correct result. Assert the command SUCCEEDS; asserting on its output would
# be asserting that something is installed, which is no longer true.
if out="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" pip list --python /usr/bin/python3 2>&1)"; then
  echo "${out:-(empty listing, as expected: no pip, no site-packages)}" | head -4
else
  echo "uv pip list failed against the bundled interpreter:"; echo "$out"; exit 1
fi

echo "== uv does pip's job natively, with no pip package present =="
# ONE shell-free invocation (this image ships no shell): --target installs without
# needing a venv or pip. Proves the dropped py-pip costs no real capability.
out="$(docker run --rm --read-only --tmpfs /tmp -e HOME=/tmp -e UV_CACHE_DIR=/tmp/uvcache \
        "$IMAGE" pip install --python /usr/bin/python3 --target /tmp/t packaging 2>&1)"
echo "$out" | tail -3
echo "$out" | grep -qiE 'packaging' || { echo "uv pip install --target failed:"; echo "$out"; exit 1; }

echo "== runs as nonroot uid 1001 =="
uid="$(docker run --rm --read-only --tmpfs /tmp --entrypoint /usr/bin/python3 "$IMAGE" -c 'import os; print(os.getuid())')"
[ "$uid" = "1001" ] || { echo "expected uid 1001, got '$uid'"; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected configured user 1001, got '$user'"; exit 1; }

echo "== entrypoint is the uv binary =="
ep="$(docker inspect "$IMAGE" --format '{{json .Config.Entrypoint}}')"
case "$ep" in
  *"/usr/bin/uv"*) : ;;
  *) echo "unexpected entrypoint: $ep"; exit 1 ;;
esac

echo "== read-only rootfs is enforced (write to / must fail) =="
if docker run --rm --read-only --tmpfs /tmp --entrypoint /usr/bin/python3 "$IMAGE" \
     -c 'open("/should-fail","w")' >/dev/null 2>&1; then
  echo "rootfs was writable, expected read-only"; exit 1
fi

echo "smoke test passed (uv ${ver#uv }, Python $PYMINOR, nonroot uid: $uid, read-only rootfs)"
