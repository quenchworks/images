#!/usr/bin/env bash
# Smoke test for a built Syncthing image. Usage: test.sh <image-ref> [version]
# Two instances on a Docker network, read-only roots with /var/syncthing on a volume, are paired
# through the REST API (API key from STGUIAPIKEY) and share one folder; a file copied into A must
# arrive on B with the same content. A REST call without the key must be refused.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
TAG="quench-syncthing-smoke-$$"
NET="$TAG-net"
KEY="smoke-$$-api-key"
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
cleanup() { docker rm -f "$TAG-a" "$TAG-b" >/dev/null 2>&1 || true; docker volume rm "$TAG-a" "$TAG-b" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; rm -rf "$WORK"; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
v="$(docker run --rm --entrypoint /usr/bin/syncthing "$IMAGE" --version)"
echo "$v"
[ -z "$WANT" ] || grep -qF "v$WANT " <<<"$v" || { echo "expected v$WANT"; exit 1; }

docker network create "$NET" >/dev/null
port=18384
for n in a b; do
  docker volume create "$TAG-$n" >/dev/null
  docker run -d --name "$TAG-$n" --network "$NET" --network-alias "$n" --read-only --tmpfs /tmp:uid=1001,gid=1001 \
    -v "$TAG-$n:/var/syncthing" -e STGUIAPIKEY="$KEY" -p "127.0.0.1:$port:8384" "$IMAGE" >/dev/null
  port=$((port + 1))
done
api() { local p=$1; shift; curl -fsS -m 10 -H "X-API-Key: $KEY" "$@" "http://127.0.0.1:$p/rest/$REST"; }
myid() { REST=system/status api "$1" | python3 -c 'import json,sys; print(json.load(sys.stdin)["myID"])'; }
for p in 18384 18385; do
  for i in $(seq 1 60); do myid "$p" >/dev/null 2>&1 && break
    [ "$i" = 60 ] && { echo "REST on $p never up"; docker logs "$TAG-a" 2>&1 | tail -20; exit 1; }; sleep 1; done
done
A="$(myid 18384)"; B="$(myid 18385)"
echo "  devices: $A / $B"
code="$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:18384/rest/system/status)"
[ "$code" = 403 ] || [ "$code" = 401 ] || { echo "REST without the key got HTTP $code"; exit 1; }
echo "  REST without the API key refused ($code)"

# each side knows the other at a fixed address; one shared folder on both
for side in "18384 $B b" "18385 $A a"; do
  set -- $side
  REST=config/devices api "$1" -X POST -H 'Content-Type: application/json' \
    -d "{\"deviceID\":\"$2\",\"name\":\"$3\",\"addresses\":[\"tcp://$3:22000\"]}" >/dev/null
  REST=config/folders api "$1" -X POST -H 'Content-Type: application/json' \
    -d "{\"id\":\"smoke\",\"label\":\"smoke\",\"path\":\"/var/syncthing/smoke\",\"rescanIntervalS\":5,\"devices\":[{\"deviceID\":\"$A\"},{\"deviceID\":\"$B\"}]}" >/dev/null
done

echo "quench-synced-$$" > "$WORK/hello.txt"
docker cp "$WORK/hello.txt" "$TAG-a:/var/syncthing/smoke/hello.txt" 2>/dev/null || {
  for _ in $(seq 1 20); do docker cp "$WORK/hello.txt" "$TAG-a:/var/syncthing/smoke/hello.txt" 2>/dev/null && break; sleep 1; done; }
REST="db/scan?folder=smoke" api 18384 -X POST >/dev/null || true
got=""
for _ in $(seq 1 90); do
  got="$(docker cp "$TAG-b:/var/syncthing/smoke/hello.txt" - 2>/dev/null | tar -xO 2>/dev/null || true)"
  [ "$got" = "quench-synced-$$" ] && break; sleep 1
done
[ "$got" = "quench-synced-$$" ] || { echo "file never reached B"; docker logs "$TAG-a" 2>&1 | tail -15; docker logs "$TAG-b" 2>&1 | tail -15; exit 1; }
echo "  file written on A arrived on B: $got"
if docker logs "$TAG-a" 2>&1 | grep -qiE 'read-only file system|panic'; then echo "crashed or wrote to the root"; exit 1; fi
echo "smoke test passed (syncthing ${WANT:-?}: two devices paired over REST, folder synced, API key enforced, read-only root; user $user)"
