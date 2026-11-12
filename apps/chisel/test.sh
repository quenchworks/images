#!/usr/bin/env bash
# Smoke test for a built chisel image. Usage: test.sh <image-ref> [version]
# Runs a chisel server with auth and a chisel client (same image) that forwards a local
# port through the tunnel to a backend container, then fetches through it. A client with
# the wrong credentials must not connect.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NET="quench-chisel-$$"; SV="quench-chisel-sv-$$"; CL="quench-chisel-cl-$$"; BAD="quench-chisel-bad-$$"; BE="quench-chisel-be-$$"
cleanup() { docker rm -f "$SV" "$CL" "$BAD" "$BE" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
ver="$(docker run --rm "$IMAGE" --version)"
echo "chisel: $ver"
[ -z "$WANT" ] || grep -qF "$WANT" <<<"$ver" || { echo "expected $WANT"; exit 1; }

docker network create "$NET" >/dev/null
docker run -d --name "$BE" --network "$NET" --entrypoint /usr/bin/caddy \
  ghcr.io/quenchworks/images/caddy:2.11.9 respond --listen :8000 quench-backend >/dev/null
docker run -d --name "$SV" --network "$NET" "$IMAGE" server --port 8080 --auth quench:quench-pass >/dev/null
docker run -d --name "$CL" --network "$NET" -p 127.0.0.1::9000 "$IMAGE" \
  client --auth quench:quench-pass "http://$SV:8080" "0.0.0.0:9000:$BE:8000" >/dev/null
port="$(docker port "$CL" 9000/tcp | head -1 | sed 's/.*://')"
for i in $(seq 1 30); do
  out="$(curl -s --max-time 3 "http://127.0.0.1:$port/" || true)"
  [ "$out" = quench-backend ] && break
  [ "$i" = 30 ] && { echo "tunnel fetch failed: $out"; docker logs "$SV" | tail; docker logs "$CL" | tail; exit 1; }
  sleep 1
done
echo "through the tunnel: $out"

docker run -d --name "$BAD" --network "$NET" "$IMAGE" \
  client --max-retry-count 1 --auth quench:wrong "http://$SV:8080" "0.0.0.0:9001:$BE:8000" >/dev/null
sleep 5
docker logs "$BAD" 2>&1 | grep -qi "authentication failed\|unable to authenticate\|handshake failed" \
  || { echo "a client with the wrong password was not refused"; docker logs "$BAD" 2>&1 | tail; exit 1; }

echo "smoke test passed ($ver, nonroot user: $user)"
