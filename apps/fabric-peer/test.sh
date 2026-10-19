#!/usr/bin/env bash
# Smoke test for a built Fabric peer image. Usage: test.sh <image-ref> [version]
# Prints the stamped version, then starts `peer node start` on the shipped sampleconfig
# (sample msp, no channels) and requires a real 200 from /healthz on the operations
# listener, plus the version over /version. Channels and chaincode need an orderer and
# crypto material per org: that is a network-level test, not this one.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="fabric-peer-test-$$"
trap 'docker rm -f "$NAME" >/dev/null 2>&1 || true' EXIT

v="$(docker run --rm "$IMAGE" peer version 2>&1)"
echo "$v"
[ -z "$WANT" ] || grep -qE "Version: $WANT\$" <<<"$v" || { echo "expected peer $WANT"; exit 1; }
grep -qE 'Commit SHA: [0-9a-f]{7}' <<<"$v" || { echo "commit not stamped"; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
for b in detect build release; do
  docker run --rm --entrypoint /opt/hyperledger/ccaas_builder/bin/$b "$IMAGE" >/dev/null 2>&1 && { echo "ccaas $b accepted no args"; exit 1; }
done
echo "ccaas builder binaries present"

# core.yaml's vm.endpoint is the Docker socket; with no socket the docker health check keeps
# /healthz at 503. Chaincode runs through the ccaas external builder, so run with vm.endpoint
# empty, as a Kubernetes deployment does. 2.5 ignores an empty CORE_VM_ENDPOINT (viper treats
# it as unset), so mount the image's own core.yaml with that one line changed.
CFG="$(mktemp -d)"; trap 'docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$CFG"' EXIT
cid="$(docker create "$IMAGE")"
docker cp "$cid:/etc/hyperledger/fabric/core.yaml" "$CFG/core.yaml" >/dev/null
docker rm "$cid" >/dev/null
sed -i 's|^\([[:space:]]*endpoint:\) unix:///var/run/docker.sock$|\1 ""|' "$CFG/core.yaml"
grep -q '^[[:space:]]*endpoint: ""$' "$CFG/core.yaml" || { echo "vm.endpoint line not found in core.yaml"; exit 1; }
chmod 0644 "$CFG/core.yaml"
docker run -d --name "$NAME" -e CORE_OPERATIONS_LISTENADDRESS=0.0.0.0:9443 \
  -v "$CFG/core.yaml:/etc/hyperledger/fabric/core.yaml:ro" \
  -p 127.0.0.1::9443 "$IMAGE" >/dev/null
port="$(docker port "$NAME" 9443/tcp | head -1 | sed 's/.*://')"
code=""
for _ in $(seq 1 60); do
  code="$(curl -s -o /tmp/peer-health.$$ -w '%{http_code}' "http://127.0.0.1:$port/healthz" || true)"
  [ "$code" = 200 ] && break
  [ "$(docker inspect -f '{{.State.Running}}' "$NAME")" = true ] || break
  sleep 1
done
if [ "$code" != 200 ]; then echo "peer /healthz never returned 200 (last: $code $(cat /tmp/peer-health.$$ 2>/dev/null))"; docker logs "$NAME" 2>&1 | tail -60; exit 1; fi
echo "healthz: $(cat /tmp/peer-health.$$)"; rm -f /tmp/peer-health.$$
ver="$(curl -fsS "http://127.0.0.1:$port/version")"
echo "version endpoint: $ver"
[ -z "$WANT" ] || grep -q "\"Version\":\"$WANT\"" <<<"$ver" || { echo "operations /version mismatch"; exit 1; }
logs="$(docker logs "$NAME" 2>&1)"
grep -q 'Started peer with ID' <<<"$logs" || { echo "peer did not report started"; tail -60 <<<"$logs"; exit 1; }
echo "smoke test passed (nonroot user: $user)"
