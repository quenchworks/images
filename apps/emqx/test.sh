#!/usr/bin/env bash
# Smoke test for a built EMQX image. Usage: test.sh <image-ref>
# Boots the broker, waits for the dashboard /status to go healthy, and confirms the
# broker runs nonroot. /status is EMQX's built-in liveness endpoint (200 when the
# node is up), so it doubles as the chart's readiness probe.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-emqx-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

# must run as the nonroot emqx user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "starting broker (single node)"
docker run -d --name "$NAME" -p 18083:18083 -p 1883:1883 "$IMAGE" >/dev/null

echo "waiting for dashboard /status"
ok=""
for i in $(seq 1 60); do
  code="$(docker run --rm --network host curlimages/curl:latest -s -o /dev/null -w '%{http_code}' http://127.0.0.1:18083/status 2>/dev/null || true)"
  if [ "$code" = "200" ]; then ok=1; break; fi
  sleep 2
done
[ -n "$ok" ] || { echo "EMQX /status never returned 200"; docker logs "$NAME" | tail -40 || true; exit 1; }

# version sanity via emqx_ctl. Capture first, THEN parse: piping emqx_ctl
# straight into `awk ... exit` SIGPIPEs emqx_ctl when awk quits on the first
# match, and under pipefail that fails the whole script with 141.
broker_out="$(docker exec "$NAME" /opt/emqx/bin/emqx_ctl broker 2>/dev/null || true)"
ver="$(awk -F': ' '/version/{print $2; exit}' <<<"$broker_out")"
echo "broker version: ${ver:-unknown}"

echo "smoke test passed (nonroot user: $user, dashboard /status healthy)"
