#!/usr/bin/env bash
# Smoke test for a built Nebula image. Usage: test.sh <image-ref> [version]
# nebula-cert from the image makes a CA, a lighthouse and a host certificate. A lighthouse
# and a host (both with tun.disabled, so no NET_ADMIN) start from those, and the host must
# complete a handshake with the lighthouse over UDP: the PKI, the protocol and the
# lighthouse path all work. A host signed by another CA must be refused. 1.12 binds a
# control socket in /var/run/nebula, so /run is a tmpfs here as it is an emptyDir in a pod.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
TAG="nebula-smoke-$$"
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
cleanup() { docker rm -f "$TAG-lh" "$TAG-host" "$TAG-rogue" >/dev/null 2>&1 || true; docker network rm "$TAG" >/dev/null 2>&1 || true; rm -rf "$WORK"; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
v="$(docker run --rm "$IMAGE" -version 2>&1)"
echo "$v"
[ -z "$WANT" ] || grep -qF "$WANT" <<<"$v" || { echo "expected $WANT"; exit 1; }

chmod 0777 "$WORK"
cert() { docker run --rm -v "$WORK:/c" -w /c --entrypoint /usr/bin/nebula-cert "$IMAGE" "$@"; }
cert ca -name smoke-ca
cert sign -name lh -ip 192.168.100.1/24
cert sign -name host -ip 192.168.100.2/24
cert ca -name rogue-ca -out-crt rogue-ca.crt -out-key rogue-ca.key
cert sign -name rogue -ip 192.168.100.3/24 -ca-crt rogue-ca.crt -ca-key rogue-ca.key
conf() { # name lighthouse?
  cat <<YML
pki: { ca: /c/ca.crt, cert: /c/$1.crt, key: /c/$1.key }
listen: { host: 0.0.0.0, port: 4242 }
tun: { disabled: true }
lighthouse:
  am_lighthouse: $2
  hosts: $( [ "$2" = true ] && echo '[]' || echo '["192.168.100.1"]' )
static_host_map: { "192.168.100.1": ["$TAG-lh:4242"] }
firewall:
  outbound: [{ port: any, proto: any, host: any }]
  inbound: [{ port: any, proto: any, host: any }]
YML
}
conf lh true > "$WORK/lh.yml"; conf host false > "$WORK/host.yml"; conf rogue false > "$WORK/rogue.yml"
sed -i 's|/c/ca.crt|/c/rogue-ca.crt|' "$WORK/rogue.yml"
chmod -R a+rX "$WORK"

docker network create "$TAG" >/dev/null
docker run -d --name "$TAG-lh" --network "$TAG" --read-only --tmpfs /run:uid=1001,gid=1001 -v "$WORK:/c:ro" "$IMAGE" -config /c/lh.yml >/dev/null
sleep 2
docker run -d --name "$TAG-host" --network "$TAG" --read-only --tmpfs /run:uid=1001,gid=1001 -v "$WORK:/c:ro" "$IMAGE" -config /c/host.yml >/dev/null
docker run -d --name "$TAG-rogue" --network "$TAG" --read-only --tmpfs /run:uid=1001,gid=1001 -v "$WORK:/c:ro" "$IMAGE" -config /c/rogue.yml >/dev/null
ok=0
for _ in $(seq 1 30); do
  grep -q 'Handshake message received' <<<"$(docker logs "$TAG-host" 2>&1)" && { ok=1; break; }
  sleep 1
done
[ "$ok" = 1 ] || { echo "host never completed a handshake"; docker logs "$TAG-lh" 2>&1 | tail -15; docker logs "$TAG-host" 2>&1 | tail -15; exit 1; }
echo "host <-> lighthouse handshake complete"
sleep 3
# logs captured once and grepped as here-strings: a piped grep -q under pipefail turns an
# early match into a failure once the log is long
rogue="$(docker logs "$TAG-rogue" 2>&1)"; lh="$(docker logs "$TAG-lh" 2>&1)"
if grep -q 'Handshake message received' <<<"$rogue"; then echo "a host from another CA was let in"; exit 1; fi
grep -qiE 'could not find ca|certificate validation failed|not signed by|could not verify|untrusted' <<<"$lh" \
  || { echo "lighthouse did not report rejecting the rogue certificate:"; tail -10 <<<"$lh"; exit 1; }
echo "rogue host refused"
if grep -E 'panic|read-only file system' <<<"$lh"; then echo "crashed or wrote to the root"; exit 1; fi
echo "smoke test passed: CA and certs from nebula-cert, lighthouse handshake, foreign CA refused, user 1001"
