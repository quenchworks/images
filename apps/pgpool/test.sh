#!/usr/bin/env bash
# Smoke test for a built Pgpool-II image. Usage: test.sh <image-ref>
#
# The runtime image is minimal (no shell, no psql) by design, so the test drives
# it from the host:
#   1. `pgpool --version` reports a real version (the binary runs at all).
#   2. Start it on the baked-in default config (nonroot 1001, read-only rootfs,
#      writable tmpfs for the socket/pid dir and /tmp) and wait for :9999.
#   3. Speak the PostgreSQL v3 wire protocol at :9999 and require a real protocol
#      message back -- this is what proves pgpool ANSWERS rather than just
#      accepting TCP. With no reachable backend the reply is an ErrorResponse
#      ('E'); with one it is an AuthenticationRequest ('R'). Anything else (or
#      silence) fails.
#   4. The PCP admin listener on :9898 accepts connections.
#   5. The container is still running and configured as nonroot uid 1001.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-pgpool-smoke-$$"
PORT=9999
PCP_PORT=9898

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

# --- 1. the binary runs and reports its version ----------------------------
ver_out="$(docker run --rm --entrypoint /usr/bin/pgpool "$IMAGE" --version 2>&1)"
echo "$ver_out"
printf '%s\n' "$ver_out" | grep -qE 'pgpool-II version [0-9]+\.[0-9]+\.[0-9]+' || {
  echo "pgpool --version did not report a version"; exit 1; }

echo "starting $IMAGE"
# Baked-in /etc/pgpool/{pgpool.conf,pool_hba.conf,pcp.conf}: listen 9999 + PCP 9898,
# sockets + pidfile under /var/run/pgpool, pgpool_status/lock files under /tmp,
# logs to stderr. Read-only rootfs with writable tmpfs mounts, matching the chart.
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /var/run/pgpool:uid=1001,gid=1001 \
  --tmpfs /tmp:uid=1001,gid=1001 \
  -p "127.0.0.1:${PORT}:9999" \
  -p "127.0.0.1:${PCP_PORT}:9898" \
  "$IMAGE" >/dev/null

# --- 2 + 3. wait for a real PostgreSQL v3 answer on 9999 -------------------
# NOT a plain TCP-accept wait: docker's userland port proxy binds the published
# port and ACCEPTS immediately, before (and after) the process inside is alive, so
# "the port accepts" says nothing. Instead we send a StartupMessage and require a
# valid protocol message back. Both failure modes we actually hit are caught this
# way: pgpool still booting, and pgpool dead (a missing/unreadable pool_passwd made
# it FATAL at startup while :9999 kept accepting).
#
# StartupMessage: int32 len(21) | int32 proto(3.0 = 0x00030000) | "user\0pgpool\0" | \0
# With no reachable backend the reply is an ErrorResponse ('E'); with one it is an
# AuthenticationRequest ('R'). The socket lives in its own subshell.
handshake() {
  local port="$1"
  (
    exec 3<>"/dev/tcp/127.0.0.1/$port" || exit 1
    printf '\x00\x00\x00\x15\x00\x03\x00\x00user\x00pgpool\x00\x00' >&3
    timeout 10 dd bs=1 count=1 status=none <&3 | od -An -c | tr -d ' \n'
  ) 2>/dev/null
}

first=""
for i in $(seq 1 45); do
  first="$(handshake "$PORT" || true)"
  case "$first" in R|E) break ;; esac
  docker ps --filter "name=$NAME" --filter "status=running" --format '{{.Names}}' \
    | grep -q "$NAME" || { echo "container died during startup:"; docker logs "$NAME"; exit 1; }
  sleep 1
done
echo "first response byte from :$PORT = '${first}'"
case "$first" in
  R|E) echo "pgpool answered the PostgreSQL v3 handshake on :$PORT (message type '$first')" ;;
  *)   echo "no valid PostgreSQL protocol reply on :$PORT (got '${first}')"
       docker logs "$NAME"; exit 1 ;;
esac

# --- 4. PCP admin listener -------------------------------------------------
# pgpool binds 9999 and 9898 in the same startup path, so by now PCP must be up.
(exec 3<>"/dev/tcp/127.0.0.1/$PCP_PORT") 2>/dev/null \
  || { echo "PCP is not listening on $PCP_PORT"; docker logs "$NAME"; exit 1; }
echo "PCP admin listener is up on :$PCP_PORT"

# --- 5. still running ------------------------------------------------------
# No log-line assertion: the wording differs per release and per backend state, and
# the handshake above already proves pgpool is alive AND serving.
docker ps --filter "name=$NAME" --filter "status=running" --format '{{.Names}}' \
  | grep -q "$NAME" || { echo "container is not running"; docker logs "$NAME"; exit 1; }

# Must run as the nonroot pgpool user (uid 1001). The image has no shell, so
# inspect the configured user rather than exec'ing `id`.
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user, read-only rootfs)"
