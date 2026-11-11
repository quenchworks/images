#!/usr/bin/env bash
# Smoke test for a built OpenZiti image. Usage: test.sh <image-ref> [version]
#
# An open port proves nothing, so this test needs a working overlay control plane:
#   1. `ziti version` reports the version.
#   2. `ziti edge quickstart` on a READ-ONLY rootfs as uid 1001 (tmpfs home): the controller
#      creates its PKI and database, starts the edge API, then enrolls and runs an edge router.
#   3. The CLI logs in to the edge API with the admin password and lists the edge routers: the
#      router must report isOnline, which needs its mTLS control channel to the controller.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
NAME="quench-openziti-smoke-$$"
PW="quench-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

ver="$(docker run --rm "$IMAGE" version 2>&1)"
echo "version: $ver"
grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+' <<<"$ver" || { echo "no version"; exit 1; }
if [ -n "${2:-}" ]; then
  [ "$ver" = "v${2}" ] || { echo "expected v$2"; exit 1; }
fi

docker run -d --name "$NAME" --read-only --tmpfs /tmp --tmpfs /home/ziti:uid=1001,mode=0750 "$IMAGE" \
  edge quickstart --home /home/ziti/qs --ctrl-address 127.0.0.1 --router-address 127.0.0.1 -p "$PW" >/dev/null

routers=""
for i in $(seq 1 90); do
  if docker exec "$NAME" ziti edge login 127.0.0.1:1280 -u admin -p "$PW" -y >/dev/null 2>&1; then
    routers="$(docker exec "$NAME" ziti edge list edge-routers -j 2>/dev/null || true)"
    grep -q '"isOnline": *true' <<<"$routers" && break
  fi
  docker ps --filter "name=$NAME" --filter status=running --format '{{.Names}}' | grep -q "$NAME" \
    || { echo "container died:"; docker logs "$NAME" 2>&1 | tail -40; exit 1; }
  [ "$i" = 90 ] && { echo "no edge router came online"; echo "$routers"; docker logs "$NAME" 2>&1 | tail -40; exit 1; }
  sleep 1
done
echo "edge router online after ~${i}s: $(grep -o '"name": *"[^"]*"' <<<"$routers" | head -1)"

if docker logs "$NAME" 2>&1 | grep -qiE 'read-only file system|permission denied|"level":"fatal"|panic:'; then
  echo "errors in the logs:"; docker logs "$NAME" 2>&1 | grep -iE 'read-only|permission denied|fatal|panic' | head; exit 1
fi

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed (nonroot $user, read-only rootfs, controller + edge router online)"
