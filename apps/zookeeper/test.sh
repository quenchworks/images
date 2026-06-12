#!/usr/bin/env bash
# Smoke test for a built ZooKeeper image. Usage: test.sh <image-ref>
# Exercises the `ruok` four-letter word over the client port from the host.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-zookeeper-smoke-$$"
PORT=2181

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

# send a 4-letter word and read the reply (server closes the connection after)
fourlw() {
  exec 3<>"/dev/tcp/127.0.0.1/$PORT" || return 1
  printf '%s' "$1" >&3
  timeout 3 cat <&3
  exec 3<&- 3>&- 2>/dev/null || true
}

echo "starting $IMAGE"
docker run -d --name "$NAME" -p "127.0.0.1:${PORT}:2181" "$IMAGE" >/dev/null

# wait for the server to answer ruok with imok
for i in $(seq 1 40); do
  if [ "$(fourlw ruok 2>/dev/null)" = "imok" ]; then
    break
  fi
  [ "$i" = 40 ] && { echo "zookeeper did not become ready"; docker logs "$NAME" | tail -30; exit 1; }
  sleep 1
done
echo "ruok -> imok"

echo "checking srvr reports a mode (standalone)"
fourlw srvr 2>/dev/null | grep -iE 'Mode:|Zookeeper version' | head -2 || { echo "srvr failed"; exit 1; }

# must run as the nonroot zookeeper user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
