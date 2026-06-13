#!/usr/bin/env bash
# Smoke test for a built OpenTelemetry Collector image. Usage: test.sh <image-ref>
# Runs with a read-only rootfs (the posture the chart ships) plus a writable
# /tmp tmpfs, with a minimal config (otlp receiver + debug exporter +
# health_check extension), then asserts:
#   - `otelcol-contrib --version` prints the pinned version
#   - GET :13133/        -> 200 (health_check extension)
#   - GET :8888/metrics  -> 200 (collector's own prometheus metrics)
#   - the container runs as nonroot uid 1001
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-otelcol-smoke-$$"
WORKDIR="$(mktemp -d)"
CONFIG="${WORKDIR}/config.yaml"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$WORKDIR"; }
trap cleanup EXIT

echo "checking otelcol-contrib version"
docker run --rm "$IMAGE" --version

# Minimal config: OTLP receiver, debug exporter, health_check extension on
# :13133, and the collector's own prometheus metrics on :8888.
cat > "$CONFIG" <<'EOF'
extensions:
  health_check:
    endpoint: 0.0.0.0:13133
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318
processors:
  batch:
exporters:
  debug:
    verbosity: basic
service:
  telemetry:
    metrics:
      readers:
        - pull:
            exporter:
              prometheus:
                host: 0.0.0.0
                port: 8888
  extensions: [health_check]
  pipelines:
    traces:
      receivers: [otlp]
      processors: [batch]
      exporters: [debug]
    metrics:
      receivers: [otlp]
      processors: [batch]
      exporters: [debug]
    logs:
      receivers: [otlp]
      processors: [batch]
      exporters: [debug]
EOF
chmod 0644 "$CONFIG"

echo "starting $IMAGE (read-only rootfs; writable /tmp; :13133 health, :8888 metrics)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /tmp:rw,uid=1001,gid=1001 \
  -v "$CONFIG:/etc/otelcol-contrib/config.yaml:ro" \
  -p 127.0.0.1:13133:13133 \
  -p 127.0.0.1:8888:8888 \
  "$IMAGE" \
  --config=/etc/otelcol-contrib/config.yaml >/dev/null

# wait for the health_check extension to report 200
for i in $(seq 1 60); do
  code="$(curl -fsS -o /dev/null -w '%{http_code}' "http://127.0.0.1:13133/" 2>/dev/null || true)"
  if [ "$code" = "200" ]; then
    break
  fi
  [ "$i" = 60 ] && { echo ":13133 health_check did not become healthy"; docker logs "$NAME"; exit 1; }
  sleep 1
done
echo ":13133/ -> 200 (health_check)"

echo "checking :8888/metrics"
code="$(curl -fsS -o /dev/null -w '%{http_code}' "http://127.0.0.1:8888/metrics" 2>/dev/null || true)"
[ "$code" = "200" ] || { echo ":8888/metrics returned $code, expected 200"; docker logs "$NAME"; exit 1; }
echo ":8888/metrics -> 200"

# must run as the nonroot otel user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user, read-only rootfs, :13133 health + :8888 metrics OK)"
