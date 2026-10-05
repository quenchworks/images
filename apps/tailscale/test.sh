#!/usr/bin/env bash
# Smoke test for a built Tailscale image. Usage: test.sh <image-ref> [version]
# containerboot starts tailscaled in userspace mode as uid 1001; with no auth key the node
# must come up and wait for login, and the CLI must reach the daemon over its socket.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
NAME="quench-tailscale-smoke-$$"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

WANT="${2:-}"
check_ver() { # <binary> <args...>: the first output line must be the release version
  local bin=$1; shift
  local v; v="$(docker run --rm --entrypoint "/usr/local/bin/$bin" "$IMAGE" "$@" 2>&1 | sed -n 1p)"
  echo "$bin: $v"
  [ -z "$WANT" ] || [ "$v" = "$WANT" ] || { echo "$bin version '$v', expected $WANT"; exit 1; }
}
check_ver tailscale version     # the CLI has a version subcommand
check_ver tailscaled --version  # the daemon takes a flag

docker run -d --name "$NAME" "$IMAGE" >/dev/null
st=""
for i in $(seq 1 30); do
  st="$(docker exec "$NAME" /usr/local/bin/tailscale --socket=/tmp/tailscale/tailscaled.sock status --json 2>/dev/null | grep -o '"BackendState": *"[A-Za-z]*"' || true)"
  case "$st" in *NeedsLogin*) break ;; esac
  sleep 1
done
case "$st" in *NeedsLogin*) echo "daemon up, $st" ;; *) echo "tailscaled did not come up: '$st'"; docker logs "$NAME" | tail -20; exit 1 ;; esac

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed (user $user)"
