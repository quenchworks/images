#!/usr/bin/env bash
# Smoke test for a built hubble-ui-backend image. Usage: test.sh <image-ref> [version]
# Without a relay this checks that the backend starts on a read-only root, reads its env
# config, opens the events port for the frontend and keeps running while the relay is
# unreachable. The hubble-ui chart gate runs it against a real relay and Cilium in kind.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
NAME="hubble-ui-backend-smoke-$$"
trap 'docker rm -f "$NAME" >/dev/null 2>&1 || true' EXIT
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
docker run -d --name "$NAME" --read-only --tmpfs /tmp -p 127.0.0.1:18090:8090 \
  -e FLOWS_API_ADDR=127.0.0.1:1 "$IMAGE" >/dev/null
ok=0
for _ in $(seq 1 30); do (exec 3<>/dev/tcp/127.0.0.1/18090) 2>/dev/null && { ok=1; break; }; sleep 1; done
[ "$ok" = 1 ] || { echo "events port never opened"; docker logs "$NAME" 2>&1 | tail -20; exit 1; }
sleep 3
[ "$(docker inspect "$NAME" --format '{{.State.Running}}')" = true ] || { echo "backend exited"; docker logs "$NAME" 2>&1 | tail -20; exit 1; }
docker logs "$NAME" 2>&1 | grep -q 'FLOWS_API_ADDR' || { echo "env config not read"; docker logs "$NAME" 2>&1 | tail -20; exit 1; }
if docker logs "$NAME" 2>&1 | grep -qE 'panic:|level=fatal'; then echo "backend crashed"; docker logs "$NAME" 2>&1 | tail -20; exit 1; fi
echo "smoke test passed (hubble-ui-backend ${2:-?}, uid $user, events port open, relay retry)"
