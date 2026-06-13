#!/usr/bin/env bash
# Smoke test for a built Alertmanager image. Usage: test.sh <image-ref>
# Runs read-only rootfs + writable tmpfs for the state path + /tmp (the read-only
# posture the chart ships). Waits for /-/healthy, then asserts /-/ready,
# /api/v2/status, that the embedded UI is served at /, and that amtool works.
# Confirms the container runs as nonroot uid 1001.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-alertmanager-smoke-$$"
BASE="http://127.0.0.1:9093"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE (read-only rootfs, tmpfs /alertmanager + /tmp)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /alertmanager:rw,mode=1777 \
  --tmpfs /tmp:rw,mode=1777 \
  -p 127.0.0.1:9093:9093 \
  "$IMAGE" >/dev/null

# wait for the server to report healthy
for i in $(seq 1 30); do
  if curl -fsS "$BASE/-/healthy" >/dev/null 2>&1; then
    break
  fi
  [ "$i" = 30 ] && { echo "alertmanager did not become healthy"; docker logs "$NAME"; exit 1; }
  sleep 1
done

echo "healthy; checking /-/ready"
for i in $(seq 1 30); do
  if curl -fsS "$BASE/-/ready" >/dev/null 2>&1; then break; fi
  [ "$i" = 30 ] && { echo "alertmanager did not become ready"; docker logs "$NAME"; exit 1; }
  sleep 1
done

echo "checking /api/v2/status"
status="$(curl -fsS "$BASE/api/v2/status" 2>/dev/null || true)"
printf '%s' "$status" | grep -q '"versionInfo"' \
  || { echo "unexpected /api/v2/status body: $status"; docker logs "$NAME"; exit 1; }

echo "checking the embedded UI is served at /"
ui="$(curl -fsSL "$BASE/" 2>/dev/null || true)"
printf '%s' "$ui" | grep -qi "<!doctype html\|<html" \
  || { echo "embedded UI did not return HTML at /"; docker logs "$NAME"; exit 1; }

echo "checking amtool"
docker exec "$NAME" /usr/bin/amtool --version

# must run as the nonroot alertmanager user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "version:"; docker exec "$NAME" /usr/bin/alertmanager --version 2>&1 | head -1
echo "smoke test passed (nonroot uid: $user, read-only rootfs, healthy+ready+status+UI OK)"
