#!/usr/bin/env bash
# Smoke test for a built quench-ruby image. Usage: test.sh <image-ref> <minor>
# where <minor> is the expected Ruby minor version, e.g. 3.4.
#
# This is a LANGUAGE/BASE image: the interpreter IS the entrypoint, there is no
# long-running service to ping. So we exercise ruby + gem + bundler directly
# under a READ-ONLY rootfs and confirm the nonroot uid.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> <minor>}"
MINOR="${2:?usage: test.sh <image-ref> <minor>}"   # e.g. 3.4

echo "== ruby -v matches $MINOR (default entrypoint) =="
ver="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" -v 2>&1)"
echo "$ver"
case "$ver" in
  "ruby $MINOR."*) : ;;
  *) echo "expected 'ruby $MINOR.*', got '$ver'"; exit 1 ;;
esac

echo "== a real script runs =="
out="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" -e 'print "quench-" + "ruby"')"
[ "$out" = "quench-ruby" ] || { echo "script output wrong: '$out'"; exit 1; }

echo "== stdlib sanity (openssl uses ca-certificates-bundle) =="
docker run --rm --read-only --tmpfs /tmp "$IMAGE" \
  -e 'require "openssl"; require "json"; require "digest"; puts "stdlib ok " + OpenSSL::OPENSSL_VERSION'

echo "== gem --version works (read-only rootfs, GEM_HOME on /tmp) =="
gemver="$(docker run --rm --read-only --tmpfs /tmp --entrypoint /usr/bin/gem "$IMAGE" --version 2>&1)"
echo "gem $gemver"
case "$gemver" in
  [0-9]*) : ;;
  *) echo "gem --version gave unexpected output: '$gemver'"; exit 1 ;;
esac

echo "== bundler is present and runnable =="
# `bundle --version` may print "Bundler version X.Y.Z" (older) or a bare
# "X.Y.Z" (newer); accept either as long as a version number is present.
bver="$(docker run --rm --read-only --tmpfs /tmp --entrypoint /usr/bin/bundle "$IMAGE" --version 2>&1)"
echo "$bver"
case "$bver" in
  "Bundler version "[0-9]*|[0-9]*) : ;;
  *) echo "bundler --version gave unexpected output: '$bver'"; exit 1 ;;
esac

echo "== runs as nonroot uid 1001 =="
uid="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" -e 'print Process.uid')"
[ "$uid" = "1001" ] || { echo "expected uid 1001, got '$uid'"; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected configured user 1001, got '$user'"; exit 1; }

echo "== read-only rootfs is enforced (write to / must fail) =="
if docker run --rm --read-only --tmpfs /tmp "$IMAGE" \
     -e 'File.write("/should-fail","x")' >/dev/null 2>&1; then
  echo "rootfs was writable, expected read-only"; exit 1
fi

echo "smoke test passed (Ruby $MINOR, gem $gemver, nonroot uid: $uid, read-only rootfs)"
