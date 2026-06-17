#!/usr/bin/env bash
# Smoke test for a built quench-node image. Usage: test.sh <image-ref> <expect>
# where <expect> is the expected Node.js version prefix -- either a bare major
# (e.g. 22) or the specific published version (e.g. 22.22.3).
#
# This is a LANGUAGE/BASE image: the interpreter IS the entrypoint, there is no
# long-running service to ping. So we exercise node + npm directly under a
# READ-ONLY rootfs and confirm the nonroot uid.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> <expect>}"
EXPECT="${2:?usage: test.sh <image-ref> <expect>}"   # e.g. 22 or 22.22.3

echo "== node -v matches v$EXPECT (default entrypoint) =="
ver="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" -v 2>&1)"
echo "$ver"
# Accept an exact version match or a dotted prefix (so both a bare major like
# `20` and a specific tag like `20.20.2` validate against e.g. `v20.20.2`).
case "$ver" in
  "v$EXPECT") : ;;
  "v$EXPECT."*) : ;;
  *) echo "expected 'v$EXPECT' or 'v$EXPECT.*', got '$ver'"; exit 1 ;;
esac

echo "== a real script runs =="
out="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" -e 'process.stdout.write("quench-" + "node")')"
[ "$out" = "quench-node" ] || { echo "script output wrong: '$out'"; exit 1; }

echo "== core module sanity (crypto/tls use ca-certificates-bundle) =="
docker run --rm --read-only --tmpfs /tmp "$IMAGE" \
  -e 'const c=require("crypto"),t=require("tls"); console.log("core ok", c.createHash("sha256").update("x").digest("hex").slice(0,8))'

echo "== npm -v works (read-only rootfs, npm cache on /tmp) =="
# This is a shell-less, env-less image: /usr/bin/npm is a JS file whose
# `#!/usr/bin/env node` shebang can't be exec'd directly (no /usr/bin/env). Run
# it through the node interpreter, the way a `FROM` consumer's npm wrapper does.
npmver="$(docker run --rm --read-only --tmpfs /tmp --entrypoint /usr/bin/node "$IMAGE" /usr/bin/npm -v 2>&1)"
echo "npm $npmver"
case "$npmver" in
  [0-9]*) : ;;
  *) echo "npm -v gave unexpected output: '$npmver'"; exit 1 ;;
esac

echo "== runs as nonroot uid 1001 =="
uid="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" -e 'process.stdout.write(String(process.getuid()))')"
[ "$uid" = "1001" ] || { echo "expected uid 1001, got '$uid'"; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected configured user 1001, got '$user'"; exit 1; }

echo "== read-only rootfs is enforced (write to / must fail) =="
if docker run --rm --read-only --tmpfs /tmp "$IMAGE" \
     -e 'require("fs").writeFileSync("/should-fail","x")' >/dev/null 2>&1; then
  echo "rootfs was writable, expected read-only"; exit 1
fi

echo "smoke test passed (Node.js $MAJOR, npm $npmver, nonroot uid: $uid, read-only rootfs)"
