#!/usr/bin/env bash
# Smoke test for a built hubble-relay image. Usage: test.sh <image-ref> [version]
# Without Cilium there is no peer, so this checks the version, that the relay starts on a
# read-only root, opens the Observer (4245) and health (4222) ports, and retries the peer
# service instead of exiting. The chart gate runs it against a real Cilium in kind.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="hubble-relay-smoke-$$"
trap 'docker rm -f "$NAME" >/dev/null 2>&1 || true' EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
ver="$(docker run --rm --entrypoint /usr/bin/hubble-relay "$IMAGE" version)"
[ -z "$WANT" ] || echo "$ver" | grep -qF "$WANT" || { echo "version mismatch: $ver"; exit 1; }

docker run -d --name "$NAME" --read-only --tmpfs /tmp -p 127.0.0.1:14245:4245 -p 127.0.0.1:14222:4222 "$IMAGE" \
  --disable-server-tls --disable-client-tls --peer-service=127.0.0.1:1 >/dev/null
ok=0
for _ in $(seq 1 30); do
  if (exec 3<>/dev/tcp/127.0.0.1/14245) 2>/dev/null && (exec 3<>/dev/tcp/127.0.0.1/14222) 2>/dev/null; then ok=1; break; fi
  sleep 1
done
[ "$ok" = 1 ] || { echo "relay ports never opened"; docker logs "$NAME" 2>&1 | tail -30; exit 1; }
sleep 3
[ "$(docker inspect "$NAME" --format '{{.State.Running}}')" = true ] || { echo "relay exited"; docker logs "$NAME" 2>&1 | tail -30; exit 1; }
docker logs "$NAME" 2>&1 | grep -q 'Starting gRPC server' || { echo "no server start in the log"; docker logs "$NAME" 2>&1 | tail -30; exit 1; }
echo "smoke test passed (hubble-relay ${WANT:-?}, uid $user, ports 4245+4222 open, peer retry)"
