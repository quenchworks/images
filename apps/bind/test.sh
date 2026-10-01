#!/usr/bin/env bash
# Smoke test for a built bind image. Usage: test.sh <image-ref> [expected-version]
#
# 1. The baked default config starts named as uid 1001 and it logs "running".
# 2. With a test config holding an authoritative zone (docker cp, no network needed),
#    named answers A and SOA queries over UDP and TCP from the host.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
WANT="${2:-}"
N1="quench-bind-default-$$"; N2="quench-bind-zone-$$"
TMP="$(mktemp -d)"
cleanup() { docker rm -f "$N1" "$N2" >/dev/null 2>&1 || true; rm -rf "$TMP"; }
trap cleanup EXIT
fail() { echo "FAIL: $*"; docker logs "$2" 2>&1 | tail -20; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
ver="$(docker run --rm --entrypoint /usr/bin/named "$IMAGE" -v)"
echo "$ver"
[ -z "$WANT" ] || grep -q "^BIND $WANT " <<<"$ver" || { echo "expected BIND $WANT"; exit 1; }

docker run -d --name "$N1" "$IMAGE" >/dev/null
for i in $(seq 1 30); do grep -q 'running$' <<<"$(docker logs "$N1" 2>&1)" && break; sleep 1; done
grep -q 'running$' <<<"$(docker logs "$N1" 2>&1)" || fail "the default config did not reach running" "$N1"
echo "default config: named running"

cat > "$TMP/named.conf" <<'CONF'
options { directory "/tmp"; pid-file none; listen-on port 5353 { any; }; recursion no; allow-query { any; }; };
zone "quench.test" { type primary; file "/work/quench.test.zone"; };
CONF
cat > "$TMP/quench.test.zone" <<'ZONE'
$TTL 60
@   IN SOA ns.quench.test. admin.quench.test. 1 60 60 600 60
@   IN NS  ns.quench.test.
ns  IN A   192.0.2.1
www IN A   192.0.2.80
ZONE
chmod -R a+rX "$TMP"
docker create --name "$N2" -p 127.0.0.1:15353:5353/udp -p 127.0.0.1:15353:5353/tcp \
  --entrypoint /usr/bin/named "$IMAGE" -g -c /work/named.conf >/dev/null
docker cp "$TMP/." "$N2:/work"
docker start "$N2" >/dev/null
for i in $(seq 1 30); do
  a="$(dig +short +time=1 +tries=1 -p 15353 @127.0.0.1 www.quench.test A 2>/dev/null || true)"
  [ "$a" = 192.0.2.80 ] && break; sleep 1
done
[ "$a" = 192.0.2.80 ] || fail "UDP A query got '$a'" "$N2"
t="$(dig +short +tcp -p 15353 @127.0.0.1 quench.test SOA)"
grep -q '^ns.quench.test. admin.quench.test. 1 ' <<<"$t" || fail "TCP SOA query got '$t'" "$N2"
echo "smoke test passed (uid $user, default config runs, zone answers over UDP and TCP)"
