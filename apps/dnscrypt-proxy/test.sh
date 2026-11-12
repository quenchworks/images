#!/usr/bin/env bash
# Smoke test for a built dnscrypt-proxy image. Usage: test.sh <image-ref> [version]
# The proxy starts with one static DNS-over-HTTPS upstream (Cloudflare's published stamp), a
# cloaking rule and a blocked name. A plain UDP query client (python, so the host needs no DNS
# tools) checks the cloaked name answers locally, the blocked name is refused, and a real name
# resolves through the encrypted upstream.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="dnscrypt-smoke-$$"
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$WORK"; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
v="$(docker run --rm "$IMAGE" -version 2>&1)"
echo "dnscrypt-proxy $v"
[ -z "$WANT" ] || [ "$v" = "$WANT" ] || { echo "expected $WANT"; exit 1; }

cat > "$WORK/dnscrypt-proxy.toml" <<'TOML'
listen_addresses = ['0.0.0.0:5353']
server_names = ['cloudflare']
cloaking_rules = '/c/cloaking.txt'
[blocked_names]
blocked_names_file = '/c/blocked.txt'
[static.cloudflare]
stamp = 'sdns://AgcAAAAAAAAABzEuMC4wLjEAEmRucy5jbG91ZGZsYXJlLmNvbQovZG5zLXF1ZXJ5'
TOML
echo 'smoke.quench.test 192.0.2.10' > "$WORK/cloaking.txt"
echo 'blocked.quench.test' > "$WORK/blocked.txt"
chmod -R a+rX "$WORK"
docker run -d --name "$NAME" --read-only -v "$WORK:/c:ro" -p 127.0.0.1:15353:5353/udp "$IMAGE" \
  -config /c/dnscrypt-proxy.toml >/dev/null

# q NAME: rcode and A records of a UDP query to the proxy, e.g. "0 192.0.2.10"
q() { python3 - "$1" <<'PY'
import socket, struct, sys
name = sys.argv[1]
qn = b"".join(bytes([len(p)]) + p.encode() for p in name.split(".")) + b"\0"
s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM); s.settimeout(3)
s.sendto(struct.pack(">HHHHHH", 0x5151, 0x0100, 1, 0, 0, 0) + qn + struct.pack(">HH", 1, 1), ("127.0.0.1", 15353))
d = s.recv(4096)
rcode, an = d[3] & 15, struct.unpack(">H", d[6:8])[0]
i, ips = 12 + len(qn) + 4, []
for _ in range(an):
    i += 2 if d[i] & 0xC0 == 0xC0 else d.index(b"\0", i) + 1 - i
    t, _c, _ttl, l = struct.unpack(">HHIH", d[i:i + 10]); i += 10
    if t == 1: ips.append(socket.inet_ntoa(d[i:i + 4]))
    i += l
print(rcode, " ".join(ips))
PY
}
out=""
for _ in $(seq 1 30); do out="$(q smoke.quench.test 2>/dev/null || true)"; grep -q '192.0.2.10' <<<"$out" && break; sleep 1; done
grep -q '192.0.2.10' <<<"$out" || { echo "cloaked name not answered: $out"; docker logs "$NAME" 2>&1 | tail -20; exit 1; }
echo "cloaking: smoke.quench.test -> 192.0.2.10"
b="$(q blocked.quench.test)"
[ "${b%% *}" != 0 ] || grep -qE '^0 (0\.0\.0\.0)?$' <<<"$b" || { echo "blocked name answered: $b"; exit 1; }
echo "blocked: blocked.quench.test -> $b"
r=""
for _ in $(seq 1 20); do r="$(q one.one.one.one 2>/dev/null || true)"; grep -qE '^0 1\.(1\.1\.1|0\.0\.1)' <<<"$r" && break; sleep 1; done
grep -qE '^0 1\.(1\.1\.1|0\.0\.1)' <<<"$r" || { echo "upstream resolution failed: $r"; docker logs "$NAME" 2>&1 | tail -20; exit 1; }
echo "upstream (DoH): one.one.one.one -> ${r#0 }"
if docker logs "$NAME" 2>&1 | grep -qiE 'panic|read-only file system'; then echo "crashed or wrote to the root"; exit 1; fi
echo "smoke test passed: cloaking, blocking, DoH upstream, user 1001"
