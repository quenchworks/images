#!/usr/bin/env bash
# Smoke test for a built FRRouting image. Usage: test.sh <image-ref> [version]
# Enables bgpd with a minimal config, then checks through vtysh that the version matches,
# BGP runs with the configured AS, and zebra sees the container's interfaces.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="frr-smoke-$$"
CONF="$(mktemp -d "${TMPDIR:-$HOME}/frr-smoke.XXXX")"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$CONF"; }
trap cleanup EXIT

docker run --rm --entrypoint /bin/cat "$IMAGE" /etc/frr/daemons > "$CONF/daemons"
sed -i 's/^bgpd=no/bgpd=yes/' "$CONF/daemons"
printf '%s\n' 'frr defaults traditional' 'hostname quench-smoke' 'router bgp 64512' \
  ' bgp router-id 10.255.0.1' ' no bgp ebgp-requires-policy' ' neighbor 10.255.0.2 remote-as 64513' > "$CONF/frr.conf"
printf '%s\n' 'service integrated-vtysh-config' > "$CONF/vtysh.conf"
chmod 0644 "$CONF"/*; chmod 0777 "$CONF"

docker run -d --name "$NAME" --cap-add NET_ADMIN --cap-add NET_RAW --cap-add SYS_ADMIN \
  -v "$CONF:/etc/frr" "$IMAGE" >/dev/null
ok=0
for _ in $(seq 1 30); do
  out="$(docker exec "$NAME" vtysh -c 'show bgp summary' 2>&1 || true)"
  grep -q 'local AS number 64512' <<<"$out" && { ok=1; break; }
  sleep 1
done
[ "$ok" = 1 ] || { echo "bgpd never answered: $out"; docker logs "$NAME" | tail -30; exit 1; }
grep -q "10.255.0.2" <<<"$out" || { echo "peer missing from the summary: $out"; exit 1; }
echo "bgpd up, AS 64512, peer 10.255.0.2 configured"
ver="$(docker exec "$NAME" vtysh -c 'show version' | sed -n 's/^FRRouting \([0-9.]*\).*/\1/p' | head -1)"
echo "FRRouting $ver"
[ -z "$WANT" ] || [ "$ver" = "$WANT" ] || { echo "expected version $WANT"; exit 1; }
grep -q 'Interface eth0' <<<"$(docker exec "$NAME" vtysh -c 'show interface eth0')"
echo "zebra sees eth0"
echo "smoke test passed"
