#!/usr/bin/env bash
# Smoke test for a built PowerDNS Recursor image. Usage: test.sh <image-ref> [version]
#
# Recursing to the internet makes a test depend on the network, so this one
# gives the recursor a local zone through `recursor.auth_zones`, which it
# answers from its own cache path, then:
#   1. pdns_recursor --version reports the version.
#   2. It boots as uid 1001 on a READ-ONLY rootfs with only /tmp writable, on
#      the image's config plus the test zone.
#   3. dig over UDP and TCP gets the zone's A record back.
#   4. A name missing from the zone gets NXDOMAIN, from the zone's SOA.
#   5. rec_control ping over the control socket in /tmp gets a pong from every
#      thread.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
NAME="quench-recursor-smoke-$$"
# Host side only: 5353 is mDNS on most desktops.
PORT=15354
command -v dig >/dev/null || { echo "dig is required (dnsutils)"; exit 1; }
# Under $HOME, not /tmp: Docker Desktop cannot bind-mount /tmp paths.
work="$(mktemp -d "$HOME/.quench-recursor-test.XXXXXX")"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$work"; }
trap cleanup EXIT

ver_out="$(docker run --rm --entrypoint /usr/bin/pdns_recursor "$IMAGE" --version 2>&1 | grep -i 'Recursor' || true)"
echo "$ver_out"
grep -qE 'Recursor [0-9]+\.[0-9]+\.[0-9]+' <<<"$ver_out" || { echo "no version"; exit 1; }
if [ -n "${2:-}" ]; then grep -qF "Recursor ${2}" <<<"$ver_out" || { echo "expected $2"; exit 1; }; fi

cat > "$work/recursor.yml" <<'YML'
incoming:
  listen: ['0.0.0.0:5353']
  allow_from: ['0.0.0.0/0']
recursor:
  socket_dir: /tmp
  daemon: false
  auth_zones:
    - zone: quench.test
      file: /etc/powerdns/quench.test.zone
logging:
  disable_syslog: true
YML
cat > "$work/quench.test.zone" <<'ZONE'
$ORIGIN quench.test.
$TTL 300
@   IN SOA ns1 hostmaster 1 3600 600 86400 300
@   IN NS  ns1
ns1 IN A   192.0.2.1
www IN A   192.0.2.10
ZONE
chmod -R a+rX "$work"

docker run -d --name "$NAME" --read-only --tmpfs /tmp:rw,mode=1777 \
  -v "$work/recursor.yml:/etc/powerdns/recursor.yml:ro" \
  -v "$work/quench.test.zone:/etc/powerdns/quench.test.zone:ro" \
  -p "127.0.0.1:${PORT}:5353/udp" -p "127.0.0.1:${PORT}:5353/tcp" "$IMAGE" >/dev/null

for proto in +notcp +tcp; do
  ans=""
  for i in $(seq 1 30); do
    ans="$(dig @127.0.0.1 -p "$PORT" $proto +time=2 +tries=1 www.quench.test A 2>&1 || true)"
    grep -q '192.0.2.10' <<<"$ans" && break
    docker ps --filter "name=$NAME" --filter status=running --format '{{.Names}}' | grep -q "$NAME" \
      || { echo "recursor died during startup:"; docker logs "$NAME"; exit 1; }
    sleep 1
  done
  grep -qE '^www\.quench\.test\.\s+[0-9]+\s+IN\s+A\s+192\.0\.2\.10' <<<"$ans" || { echo "no A record over $proto:"; echo "$ans"; docker logs "$NAME"; exit 1; }
  echo "www.quench.test A 192.0.2.10 over $proto"
done

nx="$(dig @127.0.0.1 -p "$PORT" +time=2 +tries=1 missing.quench.test A 2>&1 || true)"
grep -q 'status: NXDOMAIN' <<<"$nx" || { echo "missing name was not NXDOMAIN:"; echo "$nx"; exit 1; }
echo "missing.quench.test NXDOMAIN"

pong="$(docker exec "$NAME" /usr/bin/rec_control --socket-dir=/tmp ping 2>&1 || true)"
echo "rec_control ping: $pong"
# 5.x answers once per thread ("pong worker", "pong tcpworker", "pong task").
[ -n "$pong" ] && ! grep -qv '^pong' <<<"$pong" || { echo "rec_control did not answer pong from every thread"; exit 1; }

if docker logs "$NAME" 2>&1 | grep -qE 'Read-only file system|[Pp]ermission denied|Fatal|Exiting'; then
  echo "found errors in logs:"; docker logs "$NAME"; exit 1
fi

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed (nonroot $user, read-only rootfs, answers over UDP and TCP)"
