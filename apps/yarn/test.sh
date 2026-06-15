#!/usr/bin/env bash
# Smoke test for a built quench-yarn image. Usage: test.sh <image-ref> <major>
# where <major> is the expected Yarn major version, e.g. 1 (Classic) or 4 (Berry).
#
# This is a BUILD/PACKAGE-TOOL image: the `yarn` launcher (a /bin/sh wrapper that
# exec's node) IS the entrypoint, there is no long-running service. We exercise
# yarn + the bundled node directly under a READ-ONLY rootfs and confirm the
# nonroot uid.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> <major>}"
MAJOR="${2:?usage: test.sh <image-ref> <major>}"   # e.g. 1 or 4

echo "== yarn --version matches major $MAJOR (default entrypoint) =="
ver="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" --version 2>&1)"
echo "$ver"
case "$ver" in
  "$MAJOR."*) : ;;
  *) echo "expected '$MAJOR.*', got '$ver'"; exit 1 ;;
esac

echo "== node -v works (the runtime yarn exec's) =="
nver="$(docker run --rm --read-only --tmpfs /tmp --entrypoint /usr/bin/node "$IMAGE" -v 2>&1)"
echo "node $nver"
case "$nver" in
  v[0-9]*) : ;;
  *) echo "node -v gave unexpected output: '$nver'"; exit 1 ;;
esac

echo "== /bin/sh exists (busybox backs the yarn launcher) =="
docker run --rm --read-only --tmpfs /tmp --entrypoint /bin/sh "$IMAGE" -c 'echo sh-ok' | grep -q sh-ok \
  || { echo "/bin/sh missing or broken"; exit 1; }

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

echo "smoke test passed (yarn $ver, node $nver, nonroot uid: $uid, read-only rootfs)"
