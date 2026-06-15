#!/usr/bin/env bash
# Smoke test for a built quench-bun image. Usage: test.sh <image-ref> <major>
# where <major> is the expected Bun major line, e.g. 1.
#
# This is a LANGUAGE/BASE image: `bun` IS the entrypoint, there is no
# long-running service to ping. We exercise bun (version, eval, run a TS file)
# under a READ-ONLY rootfs and confirm the nonroot uid.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> <major>}"
MAJOR="${2:?usage: test.sh <image-ref> <major>}"   # e.g. 1

echo "== bun --version starts with $MAJOR (default entrypoint) =="
ver="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" --version 2>&1)"
echo "$ver"
case "$ver" in
  "$MAJOR."*) : ;;
  *) echo "expected '$MAJOR.*', got '$ver'"; exit 1 ;;
esac

echo "== bun -e evaluates JS =="
out="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" -e 'console.log("quench-" + "bun")')"
[ "$out" = "quench-bun" ] || { echo "eval output wrong: '$out'"; exit 1; }

echo "== runs as nonroot uid 1001 =="
uid="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" -e 'console.log(process.getuid())')"
[ "$uid" = "1001" ] || { echo "expected uid 1001, got '$uid'"; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected configured user 1001, got '$user'"; exit 1; }

echo "== a real TypeScript file runs (read-only rootfs, project mount RO) =="
# The image is shell-less, so prepare the script on the HOST and bind-mount it
# read-only. We create it UNDER the checkout (not /tmp) so the bind-mount is
# portable across native docker (CI) and Docker Desktop (dev).
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT
cat > "$WORK/h.ts" <<'EOF'
const msg: string = "quench-bun-ts";
console.log(msg);
EOF
out2="$(docker run --rm --read-only --tmpfs /tmp -v "$WORK:/work:ro" -w /work "$IMAGE" run h.ts)"
[ "$out2" = "quench-bun-ts" ] || { echo "TS run output wrong: '$out2'"; exit 1; }

echo "== read-only rootfs is enforced (write to / must fail) =="
# bun does not always propagate a non-zero exit on an uncaught throw, so assert
# the write THROWS EROFS explicitly (a writable rootfs would print WROTE).
rofs="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" \
  -e 'try { require("fs").writeFileSync("/should-fail","x"); console.log("WROTE"); } catch(e){ console.log(e.code); }' 2>&1)"
[ "$rofs" = "EROFS" ] || { echo "rootfs was writable (got '$rofs'), expected read-only"; exit 1; }

echo "smoke test passed (Bun $ver, nonroot uid: $uid, read-only rootfs)"
