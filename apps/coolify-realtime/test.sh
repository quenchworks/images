#!/usr/bin/env bash
# Smoke test for a built Coolify realtime image. Usage: test.sh <image-ref>
#
# The container runs TWO supervised processes (soketi-entrypoint.sh):
#   * soketi  on :6001 -- GET /       -> 200 "OK" (healthCheck)
#                         GET /ready  -> 200 "OK"
#   * terminal on :6002 -- GET /ready -> 200 "OK"  (GET /<other> -> 404)
# We boot with a READ-ONLY rootfs + writable tmpfs for /tmp (FIFOs) and /home/coolify
# (node-pty spawns its ssh PTY with cwd=$HOME) -- the exact posture the chart ships --
# then prove BOTH services answer, prove the node-pty native addon loaded (the terminal
# server can't reach `server.listen(6002)` if `import pty` failed), and assert nonroot
# uid 1001.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-coolify-realtime-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE (read-only rootfs, tmpfs /tmp + /home/coolify)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /tmp:rw,mode=1777 \
  --tmpfs /home/coolify:rw,mode=0700,uid=1001,gid=1001 \
  -p 127.0.0.1:6001:6001 \
  -p 127.0.0.1:6002:6002 \
  "$IMAGE" >/dev/null

wait_200() {
  local url="$1" label="$2" i code
  for i in $(seq 1 60); do
    code="$(curl -s -o /dev/null -w '%{http_code}' "$url" 2>/dev/null || true)"
    if [ "$code" = "200" ]; then echo "$label OK after ${i}s"; return 0; fi
    if ! docker ps --format '{{.Names}}' | grep -q "^${NAME}$"; then
      echo "container exited early"; docker logs "$NAME"; exit 1
    fi
    sleep 1
  done
  echo "$label did not become healthy"; docker logs "$NAME"; exit 1
}

# soketi (the realtime pub/sub server) on :6001
wait_200 "http://127.0.0.1:6001/"      "soketi GET /"
wait_200 "http://127.0.0.1:6001/ready" "soketi GET /ready"
curl -fsS "http://127.0.0.1:6001/" | grep -q "OK" \
  || { echo "soketi / payload unexpected"; docker logs "$NAME"; exit 1; }

# terminal server (ws + node-pty) on :6002. Reaching server.listen(6002) at all PROVES
# `import pty from 'node-pty'` succeeded -- i.e. the native addon compiled & loaded.
wait_200 "http://127.0.0.1:6002/ready" "terminal GET /ready"
curl -fsS "http://127.0.0.1:6002/ready" | grep -q "OK" \
  || { echo "terminal /ready payload unexpected"; docker logs "$NAME"; exit 1; }
# A non-/ready path must 404 (proves the real terminal HTTP server, not something else).
code="$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:6002/nope" 2>/dev/null || true)"
[ "$code" = "404" ] || { echo "terminal GET /nope expected 404, got $code"; docker logs "$NAME"; exit 1; }

# Prove the node-pty native addon is actually present + loadable in the runtime image.
echo "node-pty check: require('node-pty') loads in the runtime container"
docker exec -w /terminal "$NAME" node -e "import('node-pty').then(m=>{if(typeof m.spawn!=='function'){console.error('no spawn');process.exit(1)}console.log('node-pty ok')})" \
  || { echo "node-pty failed to load in runtime image"; docker logs "$NAME"; exit 1; }

# Prove the runtime carries the tools the terminal server shells out to.
docker exec "$NAME" sh -c 'command -v ssh' >/dev/null \
  || { echo "ssh (openssh-client) missing from runtime image"; exit 1; }
docker exec "$NAME" sh -c 'command -v cloudflared' >/dev/null \
  || { echo "cloudflared missing from runtime image"; exit 1; }

# must run as the nonroot coolify user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "node version:"; docker exec "$NAME" node --version
echo "smoke test passed (nonroot uid $user, read-only rootfs; soketi :6001 + terminal :6002 healthy; node-pty loads; ssh + cloudflared present)"
