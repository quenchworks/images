#!/usr/bin/env bash
# Smoke test for a built longhorn-engine image. Usage: test.sh <image-ref> [version]
# 1. The engine-image DaemonSet's own command copies /usr/local/bin/longhorn into a volume,
#    and the copy must run in a foreign distro image (it is executed on the host and in
#    instance-manager pods, so it has to be static).
# 2. A replica and a controller (no frontend) run in one container; the controller must
#    see the replica RW and take a snapshot through it.
# 3. tgtd, sg_raw and longhorn-instance-manager report their versions.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="quench-lhe-smoke-$$"
VOL="quench-lhe-smoke-$$"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; docker volume rm "$VOL" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "0" ] || [ -z "$user" ] || { echo "expected root like upstream, got '$user'"; exit 1; }

ver="$(docker run --rm --entrypoint longhorn "$IMAGE" version --client-only)"
[ -z "$WANT" ] || grep -q "\"version\": \"v$WANT\"" <<<"$ver" || { echo "expected v$WANT: $ver"; exit 1; }

docker volume create "$VOL" >/dev/null
docker run --rm -v "$VOL:/data" --entrypoint /bin/bash "$IMAGE" -c \
  'diff /usr/local/bin/longhorn /data/longhorn > /dev/null 2>&1; if [ $? -ne 0 ]; then cp -p /usr/local/bin/longhorn /data/ && echo installed; fi'
copied="$(docker run --rm -v "$VOL:/data" python:3.12-slim /data/longhorn version --client-only)"
grep -q '"clientVersion"' <<<"$copied" || { echo "copied longhorn binary does not run outside the image: $copied"; exit 1; }

docker run -d --name "$NAME" --entrypoint /bin/bash "$IMAGE" -c \
  'mkdir -p /vol && (longhorn replica /vol --size 64m --listen 0.0.0.0:9502 > /tmp/replica.log 2>&1 &) && sleep 3 &&
   exec longhorn controller smoke --frontend "" --size 64m --current-size 64m --replica tcp://localhost:9502 > /tmp/controller.log 2>&1' >/dev/null
for i in $(seq 1 30); do
  ls_out="$(docker exec "$NAME" longhorn --url localhost:9501 ls 2>&1 || true)"
  grep -q 'tcp://localhost:9502 *RW' <<<"$ls_out" && break
  [ "$i" = 30 ] && { echo "replica never became RW: $ls_out"; docker exec "$NAME" sh -c 'tail -20 /tmp/controller.log /tmp/replica.log' || true; exit 1; }
  sleep 1
done
docker exec "$NAME" longhorn --url localhost:9501 snapshot create >/dev/null
snaps="$(docker exec "$NAME" longhorn --url localhost:9501 snapshot ls)"
[ "$(grep -cE '^[0-9a-f-]{36}$' <<<"$snaps")" -ge 1 ] || { echo "no snapshot after snapshot create: $snaps"; exit 1; }

docker run --rm --entrypoint tgtd "$IMAGE" --version
docker run --rm --entrypoint sg_raw "$IMAGE" -V 2>&1 | head -1
im="$(docker run --rm --entrypoint longhorn-instance-manager "$IMAGE" --version)"
[ -z "$WANT" ] || grep -q "v$WANT" <<<"$im" || { echo "instance-manager reports '$im'"; exit 1; }

echo "smoke test passed (longhorn-engine ${WANT:-?}: static binary runs off-image, replica+controller snapshot, tgtd, sg_raw; user ${user:-0})"
