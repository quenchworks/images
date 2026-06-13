#!/usr/bin/env bash
# Smoke test for a built Loki image. Usage: test.sh <image-ref>
# Runs with a read-only rootfs (the posture the chart ships) plus writable
# tmpfs mounts for /loki (storage) and /tmp, with a minimal single-binary
# filesystem config, then asserts:
#   - `loki -version` prints the pinned version
#   - GET /ready    -> 200 / "ready"
#   - GET /metrics  -> 200
#   - the container runs as nonroot uid 1001
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-loki-smoke-$$"
BASE="http://127.0.0.1:3100"
WORKDIR="$(mktemp -d)"
CONFIG="${WORKDIR}/loki.yaml"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$WORKDIR"; }
trap cleanup EXIT

echo "checking loki version"
docker run --rm "$IMAGE" -version

# Minimal single-binary config: filesystem storage under /loki, inmemory ring,
# anonymous, analytics off. Mirrors upstream's loki-docker-config.yaml.
cat > "$CONFIG" <<'EOF'
auth_enabled: false
server:
  http_listen_port: 3100
  grpc_listen_port: 9095
common:
  instance_addr: 127.0.0.1
  path_prefix: /loki
  storage:
    filesystem:
      chunks_directory: /loki/chunks
      rules_directory: /loki/rules
  replication_factor: 1
  ring:
    kvstore:
      store: inmemory
schema_config:
  configs:
    - from: 2020-10-24
      store: tsdb
      object_store: filesystem
      schema: v13
      index:
        prefix: index_
        period: 24h
analytics:
  reporting_enabled: false
EOF
chmod 0644 "$CONFIG"

echo "starting $IMAGE (read-only rootfs; writable /loki + /tmp; :3100)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /loki:rw,uid=1001,gid=1001 \
  --tmpfs /tmp:rw,uid=1001,gid=1001 \
  -v "$CONFIG:/etc/loki/loki.yaml:ro" \
  -p 127.0.0.1:3100:3100 \
  "$IMAGE" \
  -config.file=/etc/loki/loki.yaml >/dev/null

# wait for /ready to report ready (Loki returns 503 until startup completes)
for i in $(seq 1 60); do
  code="$(curl -fsS -o /dev/null -w '%{http_code}' "$BASE/ready" 2>/dev/null || true)"
  if [ "$code" = "200" ]; then
    break
  fi
  [ "$i" = 60 ] && { echo "loki /ready did not become ready"; docker logs "$NAME"; exit 1; }
  sleep 1
done
body="$(curl -fsS "$BASE/ready" 2>/dev/null || true)"
echo "/ready -> 200 (${body})"

echo "checking /metrics"
code="$(curl -fsS -o /dev/null -w '%{http_code}' "$BASE/metrics" 2>/dev/null || true)"
[ "$code" = "200" ] || { echo "/metrics returned $code, expected 200"; docker logs "$NAME"; exit 1; }
echo "/metrics -> 200"

# must run as the nonroot loki user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user, read-only rootfs, /ready + /metrics OK)"
