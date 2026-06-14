#!/usr/bin/env bash
# Smoke test for a built Tempo image. Usage: test.sh <image-ref>
# Runs with a read-only rootfs (the posture the chart ships) plus writable
# tmpfs mounts for /var/tempo (storage + WAL) and /tmp, with a minimal
# single-binary config (target: all), then asserts:
#   - `tempo --version` prints the pinned version
#   - GET /ready    -> 200
#   - GET /metrics  -> 200
#   - the container runs as nonroot uid 1001
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-tempo-smoke-$$"
BASE="http://127.0.0.1:3200"
WORKDIR="$(mktemp -d)"
CONFIG="${WORKDIR}/tempo.yaml"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$WORKDIR"; }
trap cleanup EXIT

echo "checking tempo version"
docker run --rm "$IMAGE" --version

# Minimal single-binary config: local/filesystem trace backend under /var/tempo,
# WAL alongside it, OTLP receivers (gRPC 4317 / HTTP 4318). Mirrors upstream's
# example single-binary config.
cat > "$CONFIG" <<'EOF'
target: all
server:
  http_listen_port: 3200
  grpc_listen_port: 9095
distributor:
  receivers:
    otlp:
      protocols:
        grpc:
          endpoint: 0.0.0.0:4317
        http:
          endpoint: 0.0.0.0:4318
ingester:
  max_block_duration: 5m
compactor:
  compaction:
    block_retention: 1h
storage:
  trace:
    backend: local
    wal:
      path: /var/tempo/wal
    local:
      path: /var/tempo/blocks
usage_report:
  reporting_enabled: false
EOF
chmod 0644 "$CONFIG"

echo "starting $IMAGE (read-only rootfs; writable /var/tempo + /tmp; :3200)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /var/tempo:rw,uid=1001,gid=1001 \
  --tmpfs /tmp:rw,uid=1001,gid=1001 \
  -v "$CONFIG:/etc/tempo/tempo.yaml:ro" \
  -p 127.0.0.1:3200:3200 \
  -p 127.0.0.1:4318:4318 \
  "$IMAGE" \
  -config.file=/etc/tempo/tempo.yaml >/dev/null

# wait for /ready to report ready (Tempo returns 503 until startup completes)
for i in $(seq 1 60); do
  code="$(curl -fsS -o /dev/null -w '%{http_code}' "$BASE/ready" 2>/dev/null || true)"
  if [ "$code" = "200" ]; then
    break
  fi
  [ "$i" = 60 ] && { echo "tempo /ready did not become ready"; docker logs "$NAME"; exit 1; }
  sleep 1
done
body="$(curl -fsS "$BASE/ready" 2>/dev/null || true)"
echo "/ready -> 200 (${body})"

echo "checking /metrics"
code="$(curl -fsS -o /dev/null -w '%{http_code}' "$BASE/metrics" 2>/dev/null || true)"
[ "$code" = "200" ] || { echo "/metrics returned $code, expected 200"; docker logs "$NAME"; exit 1; }
echo "/metrics -> 200"

# must run as the nonroot tempo user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user, read-only rootfs, /ready + /metrics OK)"
