#!/usr/bin/env bash
# Smoke test for a built backing-image-manager image. Usage: test.sh <image-ref> [version]
# Starts the daemon as longhorn-manager does (its own command, a disk with the
# longhorn-disk.cfg metafile at /data) and requires both servers to log that they
# listen and the daemon to stay up. The client-only version command checks the stamp.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="quench-bim-smoke-$$"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "0" ] || [ -z "$user" ] || { echo "expected root like upstream, got '$user'"; exit 1; }

ver="$(docker run --rm --entrypoint backing-image-manager "$IMAGE" version --client-only)"
echo "$ver"
[ -z "$WANT" ] || grep -q "v$WANT" <<<"$ver" || { echo "expected v$WANT"; exit 1; }

docker run -d --name "$NAME" --tmpfs /data --entrypoint sh "$IMAGE" -c \
  'echo "{\"diskUUID\":\"smoke-disk\"}" > /data/longhorn-disk.cfg && exec backing-image-manager --debug daemon --listen :8000 --sync-listen :8001 --disk-uuid smoke-disk' >/dev/null
for i in $(seq 1 30); do
  logs="$(docker logs "$NAME" 2>&1)"
  grep -q "Backing Image Manager listening to" <<<"$logs" && grep -q "sync server of backing Image Manager listening" <<<"$logs" && break
  [ "$i" = 30 ] && { echo "daemon did not start:"; tail -20 <<<"$logs"; exit 1; }
  sleep 1
done
sleep 3
[ "$(docker inspect -f '{{.State.Status}}' "$NAME")" = running ] || { echo "daemon exited"; docker logs "$NAME" 2>&1 | tail -20; exit 1; }
docker exec "$NAME" sh -c 'for t in qemu-img losetup dd sync; do command -v $t >/dev/null || { echo "missing $t"; exit 1; }; done'
echo "smoke test passed (backing-image-manager ${WANT:-?}: daemon and sync server up; user ${user:-0})"
