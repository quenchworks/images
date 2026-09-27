#!/usr/bin/env bash
# Smoke test for a built Squid image. Usage: test.sh <image-ref> [version]
#
# An open port proves nothing, so this test needs replies that only Squid sends:
#   1. squid -v reports the version.
#   2. The image boots as uid 1001 on a READ-ONLY rootfs with only /tmp writable,
#      the way the chart runs it.
#   3. A proxied request to a name that cannot resolve (.invalid, RFC 2606)
#      comes back as Squid's own error page: X-Squid-Error plus Server: squid.
#      That exercises the ACLs, the DNS path and the error templates, and needs
#      no network.
#   4. CONNECT to a non-443 port is refused with 403. The Safe_ports/SSL_ports
#      rules are live, so this is not an open relay.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
NAME="quench-squid-smoke-$$"
PORT=3128

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

ver_out="$(docker run --rm --entrypoint /usr/bin/squid "$IMAGE" -v 2>&1 | head -1)"
echo "$ver_out"
grep -qE 'Squid Cache: Version [0-9]+\.[0-9]+' <<<"$ver_out" || { echo "squid -v reported no version"; exit 1; }
if [ -n "${2:-}" ]; then
  grep -qF "Version ${2}" <<<"$ver_out" || { echo "expected version $2"; exit 1; }
fi

docker run -d --name "$NAME" --read-only --tmpfs /tmp:rw,mode=1777 \
  -p "127.0.0.1:${PORT}:3128" "$IMAGE" >/dev/null

hdr=""
for i in $(seq 1 45); do
  hdr="$(curl -sS -m 10 -D - -o /dev/null -x "http://127.0.0.1:${PORT}" http://quench-smoke.invalid/ 2>/dev/null || true)"
  grep -qi '^X-Squid-Error:' <<<"$hdr" && break
  docker ps --filter "name=$NAME" --filter status=running --format '{{.Names}}' | grep -q "$NAME" \
    || { echo "container died during startup:"; docker logs "$NAME"; exit 1; }
  [ "$i" = 45 ] && { echo "squid never answered a proxied request"; docker logs "$NAME"; exit 1; }
  sleep 1
done
printf '%s\n' "$hdr" | sed -n '1,10p'
grep -qi '^Server: squid' <<<"$hdr" || { echo "no 'Server: squid' header"; exit 1; }
grep -qiE '^X-Squid-Error: ERR_DNS_FAIL' <<<"$hdr" || { echo "expected ERR_DNS_FAIL"; exit 1; }

# -p makes curl send CONNECT; squid's 403 ends the tunnel, and the access log
# records it.
curl -sS -m 10 -o /dev/null -x "http://127.0.0.1:${PORT}" -p https://quench-smoke.invalid:8443/ >/dev/null 2>&1 || true
code="$(docker logs "$NAME" 2>&1 | grep -c 'TCP_DENIED/403.*CONNECT quench-smoke.invalid:8443' || true)"
[ "$code" -ge 1 ] || { echo "CONNECT to :8443 was not denied with 403"; docker logs "$NAME"; exit 1; }
echo "CONNECT to a non-SSL port denied (403)"

if docker logs "$NAME" 2>&1 | grep -qE 'read-only file system|[Pp]ermission denied|FATAL|ERROR:'; then
  echo "found read-only / permission / fatal errors in logs:"; docker logs "$NAME"; exit 1
fi

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed (nonroot $user, read-only rootfs, squid answered as a proxy)"
