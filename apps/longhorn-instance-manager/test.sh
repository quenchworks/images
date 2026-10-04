#!/usr/bin/env bash
# Smoke test for a built longhorn-instance-manager image. Usage: test.sh <image-ref> [version]
# Runs the pod's v1 command line as longhorn-manager sets it (tini, then the instance-manager
# launcher, `--debug daemon --listen :8500`), privileged with the host /dev and stand-ins for
# /sys and /lib/modules, and requires upstream's liveness probe (all four gRPC ports) to pass
# and tgtd to be up. The v2 (SPDK) path is not built into this image.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="quench-lhim-smoke-$$"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "0" ] || [ -z "$user" ] || { echo "expected root like upstream, got '$user'"; exit 1; }

ver="$(docker run --rm --entrypoint longhorn-instance-manager "$IMAGE" --version)"
echo "$ver"
[ -z "$WANT" ] || grep -q "v$WANT" <<<"$ver" || { echo "expected v$WANT"; exit 1; }

docker run -d --name "$NAME" --privileged -v /dev:/host/dev --tmpfs /host/sys --tmpfs /host/lib/modules \
  "$IMAGE" instance-manager --debug daemon --listen :8500 >/dev/null
# instance-manager-liveness-probe arrived in 1.13; older lines get the same port check by hand.
probe() {
  docker exec "$NAME" sh -c 'if command -v instance-manager-liveness-probe >/dev/null; then
    instance-manager-liveness-probe --data-engine v1 --timeout 2
  else for p in 8500 8501 8502 8503; do nc -z -w 2 localhost $p || exit 1; done; fi'
}
for i in $(seq 1 40); do
  st="$(docker inspect -f '{{.State.Status}}' "$NAME")"
  [ "$st" = exited ] || [ "$st" = dead ] && { echo "instance-manager exited"; docker logs "$NAME" 2>&1 | tail -30; exit 1; }
  probe >/dev/null 2>&1 && break
  [ "$i" = 40 ] && { echo "liveness probe never passed"; probe || true; docker logs "$NAME" 2>&1 | tail -30; exit 1; }
  sleep 1
done
docker exec "$NAME" tgtadm --lld iscsi --mode target --op show >/dev/null || { echo "tgtd is not answering"; docker logs "$NAME" 2>&1 | tail -20; exit 1; }
docker exec "$NAME" sg_raw -V >/dev/null 2>&1 || { echo "sg_raw missing"; exit 1; }

echo "smoke test passed (longhorn-instance-manager ${WANT:-?}: v1 launcher, liveness probe on 8500-8503, tgtd; user ${user:-0})"
