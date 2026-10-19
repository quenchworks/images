#!/usr/bin/env bash
# Smoke test for a built Fabric orderer image. Usage: test.sh <image-ref> [version]
# Prints the stamped version, then starts `orderer start` on the shipped sampleconfig with no
# channels. Raft orderers refuse to start without TLS, so the test turns it on with the
# sample msp's own cert and key (the only material the image ships). 2.5 defaults to a
# genesis-block bootstrap, so that line runs with BootstrapMethod none plus channel
# participation, which is how 2.5 networks without a system channel run. Requires a real
# 200 from /healthz on the operations listener and from the channel participation API.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="fabric-orderer-test-$$"
trap 'docker rm -f "$NAME" >/dev/null 2>&1 || true' EXIT

v="$(docker run --rm "$IMAGE" orderer version 2>&1)"
echo "$v"
[ -z "$WANT" ] || grep -qE "Version: $WANT\$" <<<"$v" || { echo "expected orderer $WANT"; exit 1; }
grep -qE 'Commit SHA: [0-9a-f]{7}' <<<"$v" || { echo "commit not stamped"; exit 1; }
major="$(sed -n 's/^ *Version: \([0-9]*\)\..*/\1/p' <<<"$v")"

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

M=/etc/hyperledger/fabric/msp
extra=()
[ "$major" = 2 ] && extra=(-e ORDERER_GENERAL_BOOTSTRAPMETHOD=none -e ORDERER_CHANNELPARTICIPATION_ENABLED=true)
docker run -d --name "$NAME" \
  -e ORDERER_GENERAL_TLS_ENABLED=true \
  -e ORDERER_GENERAL_TLS_PRIVATEKEY=$M/keystore/key.pem \
  -e ORDERER_GENERAL_TLS_CERTIFICATE=$M/signcerts/peer.pem \
  -e "ORDERER_GENERAL_TLS_ROOTCAS=[$M/cacerts/cacert.pem]" \
  -e ORDERER_OPERATIONS_LISTENADDRESS=0.0.0.0:8443 \
  -e ORDERER_ADMIN_LISTENADDRESS=0.0.0.0:9443 \
  "${extra[@]}" -p 127.0.0.1::8443 -p 127.0.0.1::9443 "$IMAGE" >/dev/null
ops="$(docker port "$NAME" 8443/tcp | head -1 | sed 's/.*://')"
adm="$(docker port "$NAME" 9443/tcp | head -1 | sed 's/.*://')"
code=""
for _ in $(seq 1 60); do
  code="$(curl -s -o /tmp/ord-health.$$ -w '%{http_code}' "http://127.0.0.1:$ops/healthz" || true)"
  [ "$code" = 200 ] && break
  [ "$(docker inspect -f '{{.State.Running}}' "$NAME")" = true ] || break
  sleep 1
done
if [ "$code" != 200 ]; then echo "orderer /healthz never returned 200 (last: $code)"; docker logs "$NAME" 2>&1 | tail -60; exit 1; fi
echo "healthz: $(cat /tmp/ord-health.$$)"; rm -f /tmp/ord-health.$$
ch="$(curl -fsS "http://127.0.0.1:$adm/participation/v1/channels")"
echo "channel participation: $ch"
grep -q '"channels"' <<<"$ch" || { echo "no channel list from the admin API"; exit 1; }
ver="$(curl -fsS "http://127.0.0.1:$ops/version")"
echo "version endpoint: $ver"
[ -z "$WANT" ] || grep -q "\"Version\":\"$WANT\"" <<<"$ver" || { echo "operations /version mismatch"; exit 1; }
echo "smoke test passed (nonroot user: $user)"
