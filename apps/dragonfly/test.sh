#!/usr/bin/env bash
# ============================================================================
# LICENSE WARNING — Dragonfly is BSL-1.1 (Business Source License).
#   *** NOT OSI-APPROVED OPEN SOURCE. *** Clean alternative: Valkey (BSD-3-Clause,
#   already shipped) covers the same Redis-compatible cache slot. Caution tier.
# ============================================================================
#
# Smoke test for a built Dragonfly image. Usage: test.sh <image-ref>
# Runs with a READ-ONLY rootfs + writable tmpfs (/data, /tmp), waits for the RESP
# port, then exercises the Redis protocol from a throwaway redis-cli container on a
# shared docker network (PING -> PONG, SET/GET roundtrip). Confirms nonroot uid 1001.
# The runtime image is minimal (no redis-cli by design), so the client comes from a
# separate `redis` container, matching the chart's tcpSocket-based probe model.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-dragonfly-smoke-$$"
NET="quench-dragonfly-net-$$"

cleanup() {
  docker rm -f "$NAME" >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker network create "$NET" >/dev/null

echo "starting $IMAGE (read-only rootfs, writable tmpfs for /data + /tmp)"
docker run -d --name "$NAME" --network "$NET" \
  --read-only \
  --tmpfs /data:rw,mode=1777 \
  --tmpfs /tmp:rw,mode=1777 \
  "$IMAGE" >/dev/null

# confirm nonroot uid 1001 (image has a shell; busybox id works)
UID_OUT="$(docker exec "$NAME" id -u)"
echo "runtime uid: $UID_OUT"
[ "$UID_OUT" = "1001" ] || { echo "FAIL: not running as uid 1001"; exit 1; }

rcli() { docker run --rm --network "$NET" redis:7-alpine redis-cli -h "$NAME" -p 6379 "$@"; }

echo "waiting for Dragonfly RESP (PING -> PONG)"
ok=""
for i in $(seq 1 60); do
  if [ "$(rcli PING 2>/dev/null)" = "PONG" ]; then
    ok=1; echo "PONG after ${i}s"; break
  fi
  if [ "$i" = 60 ]; then
    echo "FAIL: Dragonfly did not answer PING"; docker logs "$NAME" 2>&1 | tail -60; exit 1
  fi
  sleep 1
done

echo "SET/GET roundtrip:"
rcli SET gate quench >/dev/null
VAL="$(rcli GET gate)"
echo "  GET gate -> $VAL"
[ "$VAL" = "quench" ] || { echo "FAIL: expected 'quench', got '$VAL'"; exit 1; }

echo "INFO server (confirms it is Dragonfly):"
rcli INFO server 2>/dev/null | grep -iE 'dragonfly|server' | head -3 || true

echo "PASS: dragonfly smoke test green (nonroot uid 1001, RESP on 6379)"
