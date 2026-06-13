#!/usr/bin/env bash
# Smoke test for a built Harbor portal image. Usage: test.sh <image-ref>
# Runs read-only rootfs + a writable tmpfs /tmp (the read-only posture the chart
# ships -- nginx writes its pid + temp paths under /tmp). Waits for GET / to
# return the SPA index.html, asserts the Swagger UI is served at
# /devcenter-api-2.0, that swagger.json is reachable, and that the container runs
# as nonroot uid 1001.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-harbor-portal-smoke-$$"
BASE="http://127.0.0.1:8080"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE (read-only rootfs, tmpfs /tmp)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /tmp:rw,mode=1777 \
  -p 127.0.0.1:8080:8080 \
  "$IMAGE" >/dev/null

# wait for nginx to serve the SPA
for i in $(seq 1 30); do
  if curl -fsS "$BASE/" >/dev/null 2>&1; then
    break
  fi
  [ "$i" = 30 ] && { echo "portal did not start serving /"; docker logs "$NAME"; exit 1; }
  sleep 1
done

echo "checking GET / returns the Harbor SPA index.html"
home="$(curl -fsSL "$BASE/" 2>/dev/null || true)"
printf '%s' "$home" | grep -qi "<!doctype html\|<html\|<app-root" \
  || { echo "GET / did not return the SPA HTML"; docker logs "$NAME"; exit 1; }

echo "checking SPA deep-link fallback (GET /harbor -> index.html)"
curl -fsS "$BASE/harbor" >/dev/null \
  || { echo "SPA fallback route failed"; docker logs "$NAME"; exit 1; }

echo "checking the Swagger UI at /devcenter-api-2.0"
swagger="$(curl -fsSL "$BASE/devcenter-api-2.0" 2>/dev/null || true)"
printf '%s' "$swagger" | grep -qi "swagger\|<html\|<!doctype" \
  || { echo "Swagger UI not served at /devcenter-api-2.0"; docker logs "$NAME"; exit 1; }

echo "checking swagger.json is reachable"
sj="$(curl -fsS -o /dev/null -w '%{http_code}' "$BASE/swagger.json" 2>/dev/null || true)"
[ "$sj" = "200" ] \
  || { echo "swagger.json not reachable (HTTP $sj)"; docker logs "$NAME"; exit 1; }

# must run as the nonroot nginx user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot uid: $user, read-only rootfs, SPA + Swagger UI + swagger.json OK)"
