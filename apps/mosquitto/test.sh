#!/usr/bin/env bash
# Smoke test for a built Mosquitto image. Usage: test.sh <image-ref>
# Boots the broker, then does a real publish/subscribe round-trip over MQTT using the
# bundled mosquitto_sub/mosquitto_pub clients, and confirms the broker runs nonroot.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-mosquitto-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

# must run as the nonroot mosquitto user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "starting broker"
docker run -d --name "$NAME" -p 18831:1883 "$IMAGE" >/dev/null
# wait for the listener (mosquitto_sub via --entrypoint; the image ENTRYPOINT is the
# broker itself, so client tools must override it)
for i in $(seq 1 20); do
  if docker run --rm --network host --entrypoint mosquitto_sub "$IMAGE" -h 127.0.0.1 -p 18831 -t '$SYS/#' -C 1 -W 2 >/dev/null 2>&1; then
    break
  fi
  sleep 0.5
done

echo "pub/sub round-trip"
# subscribe (detached, exits after 1 message), publish, then read the sub's output
SUB="${NAME}-sub"
docker run -d --name "$SUB" --network host --entrypoint mosquitto_sub "$IMAGE" \
  -h 127.0.0.1 -p 18831 -t quench/test -C 1 -W 5 >/dev/null
sleep 1
docker run --rm --network host --entrypoint mosquitto_pub "$IMAGE" \
  -h 127.0.0.1 -p 18831 -t quench/test -m hello-quench
docker wait "$SUB" >/dev/null
out="$(docker logs "$SUB" 2>/dev/null | tr -d '\r\n')"
docker rm -f "$SUB" >/dev/null 2>&1 || true
echo "received: $out"
[ "$out" = "hello-quench" ] || { echo "pub/sub round-trip failed (got '$out')"; docker logs "$NAME" || true; exit 1; }

echo "smoke test passed (nonroot user: $user, MQTT pub/sub OK)"
