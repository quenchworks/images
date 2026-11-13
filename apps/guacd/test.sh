#!/usr/bin/env bash
# Smoke test for a built guacd image. Usage: test.sh <image-ref> [version]
# Starts guacd and runs the Guacamole protocol handshake for each protocol the
# image ships: a "select" must load that plugin and come back as an "args"
# instruction listing its connection parameters.
set -euo pipefail
# qw (tools/qw, Go) provides the helpers this test needs; go run reuses the build cache.
QWD="$(cd "$(dirname "$0")/../../tools/qw" && pwd)"; qw() { go -C "$QWD" run . "$@"; }

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="quench-guacd-smoke-$$"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

docker run -d --name "$NAME" --read-only -p 127.0.0.1:4822:4822 "$IMAGE" >/dev/null
for i in $(seq 1 30); do
  docker logs "$NAME" 2>&1 | grep -q "Listening on host" && break
  [ "$i" = 30 ] && { echo "guacd did not start"; docker logs "$NAME"; exit 1; }
  sleep 1
done
if [ -n "$WANT" ]; then
  docker logs "$NAME" 2>&1 | grep -q "version ${WANT} started" || { echo "expected guacd ${WANT}"; docker logs "$NAME"; exit 1; }
fi

for p in rdp vnc ssh telnet; do
  r="$(qw tlsprobe --plain --addr 127.0.0.1:4822 --send "6.select,${#p}.$p;" --print)"
  echo "$p: ${r:0:80}"
  grep -q '^4\.args,' <<<"$r" || { echo "no args reply for $p"; docker logs "$NAME" | tail -20; exit 1; }
  grep -q '8\.hostname' <<<"$r" || { echo "$p args lack hostname"; exit 1; }
done

echo "smoke test passed (guacd ${WANT:-?}: rdp, vnc, ssh, telnet; nonroot user: $user)"
