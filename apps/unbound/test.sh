#!/usr/bin/env bash
# Smoke test for a built unbound image. Usage: test.sh <image-ref> [version]
# Real resolution, not a banner: the resolver answers a recursive query, DNSSEC marks
# a signed zone authenticated (ad), and a deliberately broken signature is SERVFAIL.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="unbound-smoke-$$"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

ver="$(docker run --rm "$IMAGE" -V 2>&1 | head -1)"
echo "$ver"
[ -z "$WANT" ] || echo "$ver" | grep -q "Version ${WANT}" || { echo "expected version $WANT"; exit 1; }

docker run --rm --entrypoint /usr/bin/unbound-checkconf "$IMAGE" /etc/unbound/unbound.conf

command -v dig >/dev/null || { sudo apt-get install -y -qq dnsutils >/dev/null 2>&1 || true; }
docker run -d --name "$NAME" --read-only --tmpfs /tmp -p 127.0.0.1:15053:5053/udp -p 127.0.0.1:15053:5053/tcp "$IMAGE" >/dev/null
q() { dig @127.0.0.1 -p 15053 +time=5 +tries=2 "$@"; }
ok=0
for _ in $(seq 1 30); do
  q +short example.com A 2>/dev/null | grep -qE '^[0-9.]+$' && { ok=1; break; }
  sleep 1
done
[ "$ok" = 1 ] || { echo "no answer for example.com"; docker logs "$NAME" 2>&1 | tail -30; exit 1; }
echo "example.com A: $(q +short example.com A | head -1)"

q +dnssec isc.org A | grep -E '^;; flags:' | grep -q ' ad' || { echo "isc.org not DNSSEC-authenticated"; q +dnssec isc.org A; exit 1; }
echo "isc.org: ad (validated)"

q dnssec-failed.org A | grep -q 'status: SERVFAIL' || { echo "dnssec-failed.org was not rejected"; q dnssec-failed.org A; exit 1; }
echo "dnssec-failed.org: SERVFAIL (bad signature rejected)"
echo "smoke test passed (unbound ${WANT:-?}, uid $user)"
