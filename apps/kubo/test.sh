#!/usr/bin/env bash
# Smoke test for a built Kubo image. Usage: test.sh <image-ref> [version]
# Two nodes on a private docker network: node A adds a block, node B dials A over
# QUIC and fetches it with bitswap. That exercises the libp2p transports whose
# versions the recipe floats, not only the HTTP API.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
TAG="quench-kubo-smoke-$$"
NET="$TAG-net"

cleanup() { docker rm -f "$TAG-a" "$TAG-b" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
trap cleanup EXIT
docker network create "$NET" >/dev/null

# local-discovery drops the "server" profile's private-range dial filters, which
# would otherwise refuse the docker network's addresses.
start() {
  docker run -d --name "$TAG-$1" --network "$NET" -e IPFS_PROFILE=local-discovery \
    -p "127.0.0.1:$2:5001" -p "127.0.0.1:$3:8080" "$IMAGE" >/dev/null
}
api() { curl -fsS -X POST "http://127.0.0.1:$1/api/v0/$2"; }
field() { sed -n "s/.*\"$1\":\"\([^\"]*\)\".*/\1/p"; }
wait_api() {
  for i in $(seq 1 60); do
    api "$1" id >/dev/null 2>&1 && return 0
    sleep 1
  done
  echo "RPC API on :$1 never answered"; docker logs "$TAG-$2"; exit 1
}

echo "starting $IMAGE twice"
start a 15001 18080
start b 15002 18081
wait_api 15001 a
wait_api 15002 b

# local-discovery also turns mDNS on, and an mDNS-found TCP connection would satisfy
# the QUIC dial below without exercising QUIC. Config is read at boot, so turn mDNS off
# and restart both; the restart also proves the repo is reused (same PeerID).
peer_a="$(api 15001 id | field ID)"
for n in 15001 15002; do api "$n" "config?arg=Discovery.MDNS.Enabled&arg=false&bool=true" >/dev/null; done
docker restart "$TAG-a" "$TAG-b" >/dev/null
wait_api 15001 a
wait_api 15002 b
[ "$(api 15001 id | field ID)" = "$peer_a" ] || { echo "restart re-initialized the repo"; exit 1; }

ver="$(api 15001 version | field Version)"
echo "reported version: $ver"
case "$ver" in ""|*dev*) echo "version not stamped: '$ver'"; exit 1 ;; esac
[ -z "$WANT" ] || [ "$ver" = "$WANT" ] || { echo "expected $WANT, got $ver"; exit 1; }

payload="quenchworks-kubo-$$"
cid="$(curl -fsS -X POST -F "file=@-;filename=smoke.txt" \
  "http://127.0.0.1:15001/api/v0/add?cid-version=1&quieter=true" <<<"$payload" | field Hash)"
[ -n "$cid" ] || { echo "add returned no CID"; docker logs "$TAG-a"; exit 1; }
got="$(curl -fsS "http://127.0.0.1:18080/ipfs/$cid")"
[ "$got" = "$payload" ] || { echo "gateway A returned '$got' for $cid"; exit 1; }
echo "  node A: $cid served by its own gateway"

ip_a="$(docker inspect -f "{{(index .NetworkSettings.Networks \"$NET\").IPAddress}}" "$TAG-a")"
addr="/ip4/$ip_a/udp/4001/quic-v1/p2p/$peer_a"
api 15002 "swarm/connect?arg=$addr" >/dev/null \
  || { echo "node B could not dial A over QUIC ($addr)"; docker logs "$TAG-b" | tail -20; exit 1; }
# swarm/peers is JSON with Addr just before Peer: {"Addr":"/ip4/../udp/4001/quic-v1","Peer":"12D.."}
grep -q "/udp/4001/quic-v1\",\"Peer\":\"$peer_a\"" <<<"$(api 15002 swarm/peers)" \
  || { echo "B is connected to A, but not over QUIC"; exit 1; }
got="$(curl -fsS -m 60 -X POST "http://127.0.0.1:15002/api/v0/cat?arg=$cid")"
[ "$got" = "$payload" ] || { echo "node B fetched '$got' for $cid"; exit 1; }
echo "  node B: dialed A over QUIC and fetched $cid by bitswap"

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (version $ver, nonroot user: $user, QUIC + bitswap between two nodes, peer kept across restart)"
