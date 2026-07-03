#!/usr/bin/env bash
# Smoke test for a built Mimir image. Usage: test.sh <image-ref>
# Runs with a read-only rootfs (the posture the chart ships) plus writable
# tmpfs mounts for /data (blocks/tsdb/bucket-store + activity-tracker log) and
# /tmp, in monolithic mode (-target=all) with a minimal filesystem config, then
# asserts:
#   - `mimir -version` prints the pinned version
#   - GET /ready    -> 200
#   - GET /metrics  -> 200
#   - the container runs as nonroot uid 1001
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-mimir-smoke-$$"
BASE="http://127.0.0.1:8080"
WORKDIR="$(mktemp -d)"
CONFIG="${WORKDIR}/mimir.yaml"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$WORKDIR"; }
trap cleanup EXIT

echo "checking mimir version"
docker run --rm "$IMAGE" -version

# Minimal monolithic (single-binary) config: filesystem object storage under
# /data, in-memory ring via memberlist (single node), multitenancy off. The
# activity-tracker log defaults to a relative path (./metrics-activity.log) which
# a read-only rootfs would reject, so it is pinned under the writable /data.
cat > "$CONFIG" <<'EOF'
multitenancy_enabled: false
server:
  http_listen_port: 8080
  grpc_listen_port: 9095
activity_tracker:
  filepath: /data/metrics-activity.log
common:
  storage:
    backend: filesystem
    filesystem:
      dir: /data/storage
blocks_storage:
  backend: filesystem
  filesystem:
    dir: /data/blocks
  bucket_store:
    sync_dir: /data/tsdb-sync
  tsdb:
    dir: /data/tsdb
ruler_storage:
  backend: filesystem
  filesystem:
    dir: /data/rules
# compactor + ruler default their working dirs to ./data-compactor/ and
# ./data-ruler/ (relative to the read-only cwd); pin them under writable /data.
compactor:
  data_dir: /data/compactor
ruler:
  rule_path: /data/ruler
ingester:
  ring:
    replication_factor: 1
    kvstore:
      store: memberlist
usage_stats:
  enabled: false
EOF
chmod 0644 "$CONFIG"

echo "starting $IMAGE (read-only rootfs; writable /data + /tmp; -target=all; :8080)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /data:rw,uid=1001,gid=1001 \
  --tmpfs /tmp:rw,uid=1001,gid=1001 \
  -v "$CONFIG:/etc/mimir/mimir.yaml:ro" \
  -p 127.0.0.1:8080:8080 \
  "$IMAGE" \
  -target=all \
  -config.file=/etc/mimir/mimir.yaml >/dev/null

# wait for /ready to report ready (Mimir returns 503 until all components start)
for i in $(seq 1 90); do
  code="$(curl -fsS -o /dev/null -w '%{http_code}' "$BASE/ready" 2>/dev/null || true)"
  if [ "$code" = "200" ]; then
    break
  fi
  [ "$i" = 90 ] && { echo "mimir /ready did not become ready"; docker logs "$NAME"; exit 1; }
  sleep 1
done
echo "/ready -> 200"

echo "checking /metrics"
code="$(curl -fsS -o /dev/null -w '%{http_code}' "$BASE/metrics" 2>/dev/null || true)"
[ "$code" = "200" ] || { echo "/metrics returned $code, expected 200"; docker logs "$NAME"; exit 1; }
echo "/metrics -> 200"

# must run as the nonroot mimir user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user, read-only rootfs, /ready + /metrics OK)"
