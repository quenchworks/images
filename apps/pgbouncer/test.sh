#!/usr/bin/env bash
# Smoke test for a built PgBouncer image. Usage: test.sh <image-ref>
# The runtime image is minimal (no shell or psql client by design), so we run it
# with a minimal pgbouncer.ini mounted in, confirm it binds :6432 from the host,
# and verify it logs that it came up. Also asserts nonroot uid 1001 + read-only
# rootfs (writable socket/pid dir mounted as a tmpfs).
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-pgbouncer-smoke-$$"
PORT=6432

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE"
# Use the baked-in minimal /etc/pgbouncer/pgbouncer.ini (trust auth, listen
# 0.0.0.0:6432, socket + pid under /var/run/pgbouncer, logs to stderr). Read-only
# rootfs with a writable tmpfs for the socket/pid dir, matching the chart layout.
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /var/run/pgbouncer:uid=1001,gid=1001 \
  -p "127.0.0.1:${PORT}:6432" \
  "$IMAGE" >/dev/null

# Wait for the listener to accept TCP connections on 6432.
ready=""
for i in $(seq 1 30); do
  if (exec 3<>"/dev/tcp/127.0.0.1/$PORT") 2>/dev/null; then
    exec 3<&- 3>&- 2>/dev/null || true
    ready=1
    break
  fi
  if [ "$i" = 30 ]; then
    echo "pgbouncer did not start listening on $PORT"
    docker logs "$NAME"
    exit 1
  fi
  sleep 1
done
[ -n "$ready" ] || { echo "no listener"; exit 1; }
echo "pgbouncer is listening on :$PORT"

# Confirm the process reported it came up (PgBouncer logs "process up" at start).
logs="$(docker logs "$NAME" 2>&1)"
printf '%s\n' "$logs" | grep -qiE 'process up|listening on' || {
  echo "did not find startup line in logs:"; printf '%s\n' "$logs"; exit 1; }
echo "startup confirmed in logs"

# Process must still be running.
docker ps --filter "name=$NAME" --filter "status=running" --format '{{.Names}}' \
  | grep -q "$NAME" || { echo "container is not running"; docker logs "$NAME"; exit 1; }

# Must run as the nonroot pgbouncer user (uid 1001). The image has no shell, so
# inspect the configured user rather than exec'ing `id`.
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user, read-only rootfs)"
