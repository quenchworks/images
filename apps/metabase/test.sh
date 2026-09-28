#!/usr/bin/env bash
# Smoke test for a built Metabase image. Usage: test.sh <image-ref> [version]
#
# Runs it the way the chart does: uid 1001, READ-ONLY rootfs, writable /data
# (H2 application database, unpacked drivers) and an exec-capable /tmp (JNI
# natives such as lz4 unpack there; docker's tmpfs default is noexec). Then:
#   1. /api/health reports {"status":"ok"}: Metabase started, migrated its H2
#      application database and serves the API.
#   2. /api/session/properties reports the release's version tag.
#   3. No read-only, access-denied or native-load errors in the log.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
NAME="quench-metabase-smoke-$$"
PORT=13300
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

docker run -d --name "$NAME" --read-only \
  --tmpfs /data:rw,uid=1001,gid=1001 --tmpfs /tmp:rw,exec,mode=1777 \
  -p "127.0.0.1:${PORT}:3000" "$IMAGE" >/dev/null

h=""
for i in $(seq 1 120); do
  h="$(curl -fsS -m 5 "http://127.0.0.1:${PORT}/api/health" 2>/dev/null || true)"
  grep -q '"status":"ok"' <<<"$h" && break
  docker ps --filter "name=$NAME" --filter status=running --format '{{.Names}}' | grep -q "$NAME" \
    || { echo "metabase died during startup:"; docker logs "$NAME" 2>&1 | tail -40; exit 1; }
  [ "$i" = 120 ] && { echo "health never reported ok: $h"; docker logs "$NAME" 2>&1 | tail -40; exit 1; }
  sleep 3
done
echo "health: $h"

props="$(curl -fsS -m 10 "http://127.0.0.1:${PORT}/api/session/properties")"
tag="$(python3 -c 'import json,sys; print(json.load(sys.stdin)["version"]["tag"])' <<<"$props")"
echo "version tag: $tag"
if [ -n "${2:-}" ]; then [ "$tag" = "v${2}" ] || { echo "expected v$2"; exit 1; }; fi

if docker logs "$NAME" 2>&1 | grep -qE 'Read-only file system|AccessDenied|java.nio.file.AccessDeniedException|UnsatisfiedLinkError'; then
  echo "found write or native-load errors in logs:"; docker logs "$NAME" 2>&1 | grep -E 'Read-only|AccessDenied|UnsatisfiedLinkError' | head; exit 1
fi
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed (nonroot $user, read-only rootfs, H2 migrated, API up, $tag)"
