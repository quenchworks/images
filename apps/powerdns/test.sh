#!/usr/bin/env bash
# Smoke test for a built PowerDNS Authoritative image. Usage: test.sh <image-ref> [version]
#
# An open port proves nothing for DNS, so the test serves a real zone:
#   1. pdns_server --version reports the version.
#   2. The image boots as uid 1001 on a READ-ONLY rootfs with only the LMDB dir
#      and /tmp writable, the way the chart runs it.
#   3. pdnsutil, exec'd into the running container, creates a zone and a record
#      in the LMDB backend the server is using; pdns_control rediscover makes
#      the server load it.
#   4. dig over UDP and TCP gets that record back with the AA flag set: an
#      authoritative answer, from the backend, on both transports.
#   5. A name outside every zone is REFUSED, so the server is not a resolver.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
NAME="quench-powerdns-smoke-$$"
# Host side only: 5353 is mDNS on most desktops (avahi), so publish elsewhere.
PORT=15353
command -v dig >/dev/null || { echo "dig is required (dnsutils)"; exit 1; }

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

ver_out="$(docker run --rm --entrypoint /usr/bin/pdns_server "$IMAGE" --version 2>&1 | grep -i 'PowerDNS Authoritative Server' || true)"
echo "$ver_out"
grep -qE '[0-9]+\.[0-9]+\.[0-9]+' <<<"$ver_out" || { echo "no version"; exit 1; }
if [ -n "${2:-}" ]; then
  grep -qF "${2}" <<<"$ver_out" || { echo "expected version $2"; exit 1; }
fi

docker run -d --name "$NAME" --read-only \
  --tmpfs /var/lib/powerdns:rw,uid=1001,gid=1001 --tmpfs /tmp:rw,mode=1777 \
  -p "127.0.0.1:${PORT}:5353/udp" -p "127.0.0.1:${PORT}:5353/tcp" "$IMAGE" >/dev/null

for i in $(seq 1 30); do
  docker logs "$NAME" 2>&1 | grep -q 'Done launching threads' && break
  docker ps --filter "name=$NAME" --filter status=running --format '{{.Names}}' | grep -q "$NAME" \
    || { echo "container died during startup:"; docker logs "$NAME"; exit 1; }
  [ "$i" = 30 ] && { echo "pdns_server never finished starting"; docker logs "$NAME"; exit 1; }
  sleep 1
done

docker exec "$NAME" /usr/bin/pdnsutil --config-dir=/etc/powerdns create-zone quench.test ns1.quench.test
docker exec "$NAME" /usr/bin/pdnsutil --config-dir=/etc/powerdns add-record quench.test www.quench.test. A 3600 192.0.2.10
# The running server caches its zone list; pdnsutil wrote behind its back.
# rediscover goes over the control socket in /tmp, so this also proves
# pdns_control works on a read-only rootfs.
docker exec "$NAME" /usr/bin/pdns_control --config-dir=/etc/powerdns rediscover

for proto in +notcp +tcp; do
  ans=""
  for i in $(seq 1 10); do
    ans="$(dig @127.0.0.1 -p "$PORT" $proto +time=2 +tries=1 www.quench.test A 2>&1 || true)"
    grep -q '192.0.2.10' <<<"$ans" && break
    sleep 1
  done
  grep -qE '^www\.quench\.test\.\s+[0-9]+\s+IN\s+A\s+192\.0\.2\.10' <<<"$ans" || { echo "no A record over $proto:"; echo "$ans"; exit 1; }
  grep -qE 'flags:[^;]* aa' <<<"$ans" || { echo "answer over $proto is not authoritative:"; echo "$ans"; exit 1; }
  echo "www.quench.test A 192.0.2.10, authoritative, over $proto"
done

ref="$(dig @127.0.0.1 -p "$PORT" +time=2 +tries=1 example.com A 2>&1 || true)"
grep -q 'status: REFUSED' <<<"$ref" || { echo "a name outside every zone was not REFUSED:"; echo "$ref"; exit 1; }
echo "out-of-zone query REFUSED"

if docker logs "$NAME" 2>&1 | grep -qE 'Read-only file system|[Pp]ermission denied|Fatal|Exiting'; then
  echo "found errors in logs:"; docker logs "$NAME"; exit 1
fi

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed (nonroot $user, read-only rootfs, authoritative answers over UDP and TCP)"
