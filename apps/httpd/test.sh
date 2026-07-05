#!/usr/bin/env bash
# Smoke test for a built httpd image. Usage: test.sh <image-ref>
# Runs the image with a READ-ONLY rootfs and a writable tmpfs on /tmp (where the pid
# and runtime state live), then exercises HTTP on 8080: the default page returns 200,
# and the Server response header advertises the Apache version. Also confirms nonroot
# uid 1001.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-httpd-smoke-$$"
PORT=8080

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE (read-only rootfs + tmpfs /tmp)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /tmp:rw,mode=1777 \
  -p "127.0.0.1:${PORT}:8080" \
  "$IMAGE" >/dev/null

# wait for httpd to accept connections and serve the default page
ready=""
for i in $(seq 1 30); do
  if curl -fsS "http://127.0.0.1:${PORT}/" >/dev/null 2>&1; then
    ready=1
    break
  fi
  if [ "$i" = 30 ]; then
    echo "httpd did not become ready"
    docker logs "$NAME"
    exit 1
  fi
  sleep 1
done

echo "checking default page (GET /)"
body="$(curl -fsS "http://127.0.0.1:${PORT}/")"
printf '%s' "$body" | grep -qi 'httpd\|apache' || { echo "default page missing expected content: $body"; exit 1; }

echo "checking Server header advertises the version (200 + Apache/2.4.x)"
headers="$(curl -fsSI "http://127.0.0.1:${PORT}/")"
printf '%s' "$headers" | grep -qiE '^HTTP/[0-9.]+ 200' || { echo "expected HTTP 200, got:"; printf '%s\n' "$headers"; exit 1; }
srv="$(printf '%s' "$headers" | grep -i '^Server:' || true)"
printf '%s' "$srv" | grep -qE 'Apache/2\.4\.[0-9]+' || { echo "Server header missing Apache version: '$srv'"; exit 1; }
echo "server header: $(printf '%s' "$srv" | tr -d '\r')"

# confirm no errors writing to the read-only rootfs
if docker logs "$NAME" 2>&1 | grep -qiE 'read-only file system|permission denied'; then
  echo "detected read-only/permission errors in logs:"
  docker logs "$NAME"
  exit 1
fi

# must run as the nonroot httpd user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
