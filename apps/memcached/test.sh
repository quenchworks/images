#!/usr/bin/env bash
# Smoke test for a built memcached image. Usage: test.sh <image-ref>
# The runtime image is minimal (no shell or client by design), so exercise the
# ASCII protocol over a published port from the host using bash /dev/tcp.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-memcached-smoke-$$"
PORT=11211

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

# Send an ASCII command and read the reply line by line until the protocol
# terminator (passed in $2 as a case glob) or a short read timeout. memcached
# keeps the connection open after a reply, so we must stop on the terminator
# rather than read to EOF.
mc_io() {
  local payload="$1" term="$2" line out=""
  exec 3<>"/dev/tcp/127.0.0.1/$PORT" || return 1
  printf '%b' "$payload" >&3
  while IFS= read -t 3 -r line <&3; do
    line="${line%$'\r'}"
    out+="${line}"$'\n'
    case "$line" in $term) break ;; esac
  done
  exec 3<&- 3>&- 2>/dev/null || true
  printf '%s' "$out"
}

echo "starting $IMAGE"
docker run -d --name "$NAME" --read-only --tmpfs /tmp -p "127.0.0.1:${PORT}:11211" "$IMAGE" >/dev/null

# wait for the server to accept connections and answer the version command
ver=""
for i in $(seq 1 30); do
  if ver="$(mc_io 'version\r\n' 'VERSION*' 2>/dev/null)" && printf '%s' "$ver" | grep -q '^VERSION'; then
    break
  fi
  [ "$i" = 30 ] && { echo "memcached did not become ready"; docker logs "$NAME"; exit 1; }
  sleep 1
done
echo "version: $(printf '%s' "$ver" | head -1)"

echo "checking SET"
mc_io 'set qw 0 0 5\r\nhello\r\n' 'STORED' | grep -q 'STORED' || { echo "SET failed"; exit 1; }

echo "checking GET"
got="$(mc_io 'get qw\r\n' 'END')"
printf '%s' "$got" | grep -q 'hello' || { echo "GET failed: $got"; exit 1; }

# must run as the nonroot memcached user (uid 1001). The image has no shell by
# design, so check the configured user rather than exec'ing `id`.
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
