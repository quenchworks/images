#!/usr/bin/env bash
# Smoke test for the built quench-static base. Usage: test.sh <image-ref>
#
# This is a SHELL-LESS base (no /bin/sh, no package manager, no coreutils), so
# we cannot `docker run <img> <cmd>` to poke around inside it -- there is no
# command to run. Instead we verify it two ways:
#
#   1) Filesystem shape + identity, statically, from OUTSIDE the container:
#      export the rootfs and `docker inspect` the config. This asserts that the
#      shell is ABSENT, the CA bundle is PRESENT, and the configured user is the
#      nonroot uid 1001 -- without needing anything runnable in the image.
#
#   2) End-to-end "it actually runs a static binary": we build a tiny static
#      `hello` in a throwaway builder, COPY it onto the base via a derived
#      image, and run it under a READ-ONLY rootfs. A clean run proves the base
#      can host a static Go/Rust binary, that the nonroot uid can exec, and that
#      a read-only rootfs is fine.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"; docker rm -f quench-static-fs >/dev/null 2>&1 || true' EXIT

echo "== export rootfs for static filesystem assertions =="
# The base ships no entrypoint/cmd, so `docker create` needs a dummy command to
# satisfy the daemon. It is NEVER run -- we only `docker export` the rootfs.
docker create --name quench-static-fs "$IMAGE" /nonexistent >/dev/null
docker export quench-static-fs | tar -tf - > "$WORK/files.txt"

echo "== there is NO shell in the image =="
# Any of these would indicate a shell snuck in.
if grep -E '^(\./)?(bin|usr/bin)/(sh|bash|ash|dash|busybox)$' "$WORK/files.txt"; then
  echo "FAIL: a shell is present in the static base"; exit 1
fi
echo "ok: no /bin/sh, /bin/bash, busybox, etc."

echo "== there is NO package manager (apk) =="
if grep -E '^(\./)?(sbin|usr/bin|usr/sbin)/apk$' "$WORK/files.txt"; then
  echo "FAIL: apk is present in the static base"; exit 1
fi
echo "ok: no apk"

echo "== CA certificate bundle is present at /etc/ssl/certs =="
grep -qE '^(\./)?etc/ssl/certs/ca-certificates\.crt$' "$WORK/files.txt" \
  || { echo "FAIL: ca-certificates.crt missing"; exit 1; }
echo "ok: /etc/ssl/certs/ca-certificates.crt present"

echo "== tzdata is present (timezone-aware binaries) =="
grep -qE '^(\./)?usr/share/zoneinfo/UTC$' "$WORK/files.txt" \
  || { echo "FAIL: zoneinfo/UTC missing"; exit 1; }
echo "ok: /usr/share/zoneinfo present"

echo "== configured user is nonroot uid 1001 =="
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "FAIL: expected configured user 1001, got '$user'"; exit 1; }
echo "ok: Config.User = 1001"

echo "== /etc/passwd carries the nonroot uid 1001 entry =="
docker export quench-static-fs | tar -xO ./etc/passwd 2>/dev/null > "$WORK/passwd" \
  || docker export quench-static-fs | tar -xO etc/passwd > "$WORK/passwd"
grep -qE ':x?:1001:1001:' "$WORK/passwd" \
  || { echo "FAIL: no uid 1001 in /etc/passwd"; cat "$WORK/passwd"; exit 1; }
echo "ok: nonroot:...:1001:1001 in /etc/passwd"

echo "== end-to-end: a static binary runs on the base, read-only, nonroot =="
# Build a fully static `hello` with musl-gcc in a throwaway builder, then layer
# it onto the base. No shell is used inside the final image -- the binary IS
# the entrypoint.
cat > "$WORK/hello.c" <<'EOF'
#include <unistd.h>
#include <stdio.h>
int main(void) {
  printf("quench-static-ok uid=%d\n", (int)getuid());
  return 0;
}
EOF

cat > "$WORK/Dockerfile" <<EOF
# Build a fully static binary with musl in a throwaway builder.
FROM alpine:3 AS sbuild
RUN apk add --no-cache musl-dev gcc
COPY hello.c /hello.c
RUN gcc -static -O2 -o /hello /hello.c && /hello

FROM $IMAGE
COPY --from=sbuild /hello /hello
ENTRYPOINT ["/hello"]
EOF

docker build -t quench-static-hello:local "$WORK" >/dev/null
out="$(docker run --rm --read-only --tmpfs /tmp quench-static-hello:local)"
echo "binary output: [$out]"
case "$out" in
  "quench-static-ok uid=1001"*) : ;;
  *) echo "FAIL: static binary did not run as uid 1001 on a read-only rootfs: '$out'"; exit 1 ;;
esac

echo "smoke test passed (static base: shell-less, ca-certs + tzdata present, nonroot uid 1001, runs static binaries read-only)"
