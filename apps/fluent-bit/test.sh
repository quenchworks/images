#!/usr/bin/env bash
# Smoke test for a built Fluent Bit image. Usage: test.sh <image-ref>
# Runs the image with a READ-ONLY rootfs and a writable tmpfs on /tmp, then exercises
# the HTTP monitoring server on :2020: /api/v1/health and / both return 200, and
# /api/v1/metrics is reachable. Also confirms nonroot uid 1001.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-fluent-bit-smoke-$$"
PORT=2020

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE (read-only rootfs + tmpfs /tmp)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /tmp:rw,mode=1777 \
  -p "127.0.0.1:${PORT}:2020" \
  "$IMAGE" >/dev/null

# wait for the HTTP monitoring server to come up
ready=""
for i in $(seq 1 30); do
  if curl -fsS "http://127.0.0.1:${PORT}/api/v1/health" >/dev/null 2>&1; then
    ready=1
    break
  fi
  if [ "$i" = 30 ]; then
    echo "fluent-bit did not become ready"
    docker logs "$NAME"
    exit 1
  fi
  sleep 1
done

echo "checking /api/v1/health"
code="$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:${PORT}/api/v1/health")"
[ "$code" = "200" ] || { echo "/api/v1/health returned $code"; docker logs "$NAME"; exit 1; }

echo "checking / (root)"
code="$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:${PORT}/")"
[ "$code" = "200" ] || { echo "/ returned $code"; docker logs "$NAME"; exit 1; }

echo "checking /api/v1/metrics"
# metrics register after the first flush, so allow a short retry window.
code=""
for i in $(seq 1 10); do
  code="$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:${PORT}/api/v1/metrics")"
  [ "$code" = "200" ] && break
  sleep 1
done
[ "$code" = "200" ] || { echo "/api/v1/metrics returned $code"; docker logs "$NAME"; exit 1; }

# confirm no errors writing to the read-only rootfs
if docker logs "$NAME" 2>&1 | grep -qiE 'read-only file system|permission denied'; then
  echo "detected read-only/permission errors in logs:"
  docker logs "$NAME"
  exit 1
fi

# must run as the nonroot fluent-bit user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
