#!/usr/bin/env bash
# Smoke test for a built quench-deno image. Usage: test.sh <image-ref> <major>
# where <major> is the expected Deno major line, e.g. 2.
#
# This is a LANGUAGE/BASE image: `deno` IS the entrypoint, there is no
# long-running service to ping. We exercise deno (version, eval, run a TS file)
# under a READ-ONLY rootfs and confirm the nonroot uid.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> <major>}"
MAJOR="${2:?usage: test.sh <image-ref> <major>}"   # e.g. 2

echo "== deno --version reports major $MAJOR (default entrypoint) =="
ver="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" --version 2>&1)"
echo "$ver"
case "$ver" in
  "deno $MAJOR."*) : ;;
  *) echo "expected 'deno $MAJOR.*', got '$ver'"; exit 1 ;;
esac

echo "== deno eval runs TS (writable DENO_DIR on tmpfs) =="
out="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" eval 'console.log("quench-" + "deno")')"
[ "$out" = "quench-deno" ] || { echo "eval output wrong: '$out'"; exit 1; }

echo "== runs as nonroot uid 1001 =="
uid="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" eval 'console.log(Deno.uid())')"
[ "$uid" = "1001" ] || { echo "expected uid 1001, got '$uid'"; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected configured user 1001, got '$user'"; exit 1; }

echo "== a real TypeScript file runs (read-only rootfs, project mount RO) =="
# Shell-less image: prepare the script on the HOST and bind-mount it read-only,
# created UNDER the checkout for portable mounts (CI + Docker Desktop).
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT
cat > "$WORK/h.ts" <<'EOF'
const msg: string = "quench-deno-ts";
console.log(msg);
EOF
out2="$(docker run --rm --read-only --tmpfs /tmp -v "$WORK:/work:ro" -w /work "$IMAGE" run h.ts)"
[ "$out2" = "quench-deno-ts" ] || { echo "TS run output wrong: '$out2'"; exit 1; }

echo "== read-only rootfs is enforced (write to / must fail) =="
if docker run --rm --read-only --tmpfs /tmp "$IMAGE" \
     eval 'Deno.writeTextFileSync("/should-fail","x")' >/dev/null 2>&1; then
  echo "rootfs was writable, expected read-only"; exit 1
fi

echo "smoke test passed (Deno $MAJOR, nonroot uid: $uid, read-only rootfs)"
