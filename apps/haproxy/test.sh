#!/usr/bin/env bash
# Smoke test for a built HAProxy image. Usage: test.sh <image-ref>
# Runs the image with a READ-ONLY rootfs and writable tmpfs on /var/lib/haproxy (pid +
# admin socket) and /tmp, with a minimal config (a stats/health frontend on :8404 and
# an http frontend :8080 -> a tiny always-200 backend), then exercises HTTP:
#   - :8404/healthz returns 200 (monitor-uri)
#   - :8404/stats   returns 200 (built-in stats page)
#   - :8080/        returns 200 (http frontend -> backend)
# Also confirms nonroot uid 1001.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-haproxy-smoke-$$"
CFG="$(mktemp)"
STATSPORT=8404
HTTPPORT=8080

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -f "$CFG"; }
trap cleanup EXIT

# Minimal config with a real always-200 backend so the http frontend serves 200.
cat > "$CFG" <<'EOF'
global
    log stdout format raw local0
    maxconn 1024
    stats socket /var/lib/haproxy/admin.sock mode 660 level admin

defaults
    log     global
    mode    http
    option  httplog
    timeout connect 5s
    timeout client  50s
    timeout server  50s

frontend stats
    bind *:8404
    stats enable
    stats uri /stats
    monitor-uri /healthz

frontend http_in
    bind *:8080
    default_backend app

backend app
    # http-request return makes HAProxy answer 200 itself (no upstream server needed).
    http-request return status 200 content-type "text/plain" string "quench-haproxy ok"
EOF

echo "starting $IMAGE (read-only rootfs + tmpfs /var/lib/haproxy,/tmp)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /var/lib/haproxy:rw,mode=1777 \
  --tmpfs /tmp:rw,mode=1777 \
  -v "$CFG:/etc/haproxy/haproxy.cfg:ro" \
  -p "127.0.0.1:${STATSPORT}:8404" \
  -p "127.0.0.1:${HTTPPORT}:8080" \
  "$IMAGE" >/dev/null

# wait for the stats/health frontend to accept connections
ready=""
for i in $(seq 1 30); do
  if curl -fsS "http://127.0.0.1:${STATSPORT}/healthz" >/dev/null 2>&1; then
    ready=1
    break
  fi
  if [ "$i" = 30 ]; then
    echo "haproxy did not become ready"
    docker logs "$NAME"
    exit 1
  fi
  sleep 1
done

echo "checking /healthz (monitor-uri)"
curl -fsS "http://127.0.0.1:${STATSPORT}/healthz" >/dev/null || { echo "healthz failed"; docker logs "$NAME"; exit 1; }

echo "checking /stats"
stats="$(curl -fsS "http://127.0.0.1:${STATSPORT}/stats")"
printf '%s' "$stats" | grep -qi 'Statistics Report\|HAProxy' || { echo "stats page missing expected content"; exit 1; }
echo "stats page OK"

echo "checking http frontend (GET / on :${HTTPPORT})"
body="$(curl -fsS "http://127.0.0.1:${HTTPPORT}/")"
printf '%s' "$body" | grep -qi 'quench-haproxy ok' || { echo "http frontend missing expected content: $body"; exit 1; }
echo "http frontend OK"

# confirm no errors writing to the read-only rootfs
if docker logs "$NAME" 2>&1 | grep -qiE 'read-only file system|permission denied'; then
  echo "detected read-only/permission errors in logs:"
  docker logs "$NAME"
  exit 1
fi

# must run as the nonroot haproxy user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
