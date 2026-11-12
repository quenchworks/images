#!/usr/bin/env bash
# Smoke test for a built 3proxy image. Usage: test.sh <image-ref> [version]
# Runs 3proxy with a one-user config and fetches from a backend container through the HTTP
# proxy and the SOCKS5 proxy; a request without credentials must be refused (407). Also
# checks the shipped default config starts and refuses everyone (no users configured).
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
NET="quench-3proxy-$$"; PX="quench-3proxy-smoke-$$"; BE="quench-3proxy-be-$$"; DEF="quench-3proxy-def-$$"
WORK="$(mktemp -d)"
cleanup() { docker rm -f "$PX" "$BE" "$DEF" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; rm -rf "$WORK"; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

docker network create "$NET" >/dev/null
docker run -d --name "$BE" --network "$NET" --entrypoint /usr/bin/caddy \
  ghcr.io/quenchworks/images/caddy:2.11.9 respond --listen :8000 quench-backend >/dev/null

printf '%s\n' 'log' 'users quench:CL:quench-pass' 'auth strong' 'allow quench' \
  'proxy -p3128' 'socks -p1080' > "$WORK/3proxy.cfg"
chmod 0644 "$WORK/3proxy.cfg"
docker create --name "$PX" --network "$NET" -p 127.0.0.1::3128 -p 127.0.0.1::1080 "$IMAGE" /tmp/3proxy.cfg >/dev/null
docker cp "$WORK/3proxy.cfg" "$PX:/tmp/3proxy.cfg"
docker start "$PX" >/dev/null
hp="$(docker port "$PX" 3128/tcp | head -1 | sed 's/.*://')"
sp="$(docker port "$PX" 1080/tcp | head -1 | sed 's/.*://')"
T="http://$BE:8000/"
for i in $(seq 1 20); do
  out="$(curl -sS -x "http://quench:quench-pass@127.0.0.1:$hp" "$T" 2>/dev/null || true)"
  [ "$out" = quench-backend ] && break
  [ "$i" = 20 ] && { echo "HTTP proxy fetch failed: $out"; docker logs "$PX" | tail -20; exit 1; }
  sleep 1
done
echo "http proxy: $out"
code="$(curl -s -o /dev/null -w '%{http_code}' -x "http://127.0.0.1:$hp" "$T")"
[ "$code" = 407 ] || { echo "no credentials got HTTP $code, want 407"; exit 1; }
out="$(curl -sS --socks5-hostname "127.0.0.1:$sp" -U quench:quench-pass "$T")"
[ "$out" = quench-backend ] || { echo "SOCKS5 fetch failed: $out"; exit 1; }
echo "socks5: $out"

# the shipped default config: starts, and lets nobody through
docker run -d --name "$DEF" --network "$NET" -p 127.0.0.1::3128 "$IMAGE" >/dev/null
dp="$(docker port "$DEF" 3128/tcp | head -1 | sed 's/.*://')"
sleep 2
code="$(curl -s -o /dev/null -w '%{http_code}' -x "http://127.0.0.1:$dp" "$T" || true)"
[ "$code" = 407 ] || { echo "default config let a request through (HTTP $code)"; docker logs "$DEF" | tail; exit 1; }

echo "smoke test passed (nonroot user: $user)"
