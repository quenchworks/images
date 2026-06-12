#!/usr/bin/env bash
# Smoke test for a built VictoriaMetrics image. Usage: test.sh <image-ref>
# Runs read-only rootfs + writable tmpfs for the storage dir (the read-only
# posture the chart ships), waits for /health, writes a sample via the
# Prometheus import API, then queries it back and asserts the value.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-victoriametrics-smoke-$$"
BASE="http://127.0.0.1:8428"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE (read-only rootfs, tmpfs /storage)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /storage:rw,mode=1777 \
  -p 127.0.0.1:8428:8428 \
  "$IMAGE" >/dev/null

# wait for the HTTP API to report healthy
for i in $(seq 1 30); do
  if [ "$(curl -fsS "$BASE/health" 2>/dev/null || true)" = "OK" ]; then
    break
  fi
  [ "$i" = 30 ] && { echo "victoria-metrics did not become healthy"; docker logs "$NAME"; exit 1; }
  sleep 1
done

echo "healthy; importing a sample via the Prometheus import API"
# No explicit timestamp: VM stamps the sample at ingestion time, so it lands
# inside the query range window below.
curl -fsS -X POST "$BASE/api/v1/import/prometheus" \
  --data-binary "qw_smoke_metric 123" >/dev/null

# flush in-memory parts to make the sample immediately queryable
curl -fsS "$BASE/internal/force_flush" >/dev/null 2>&1 || true

echo "querying it back"
# Use a range selector ([5m]) rather than an instant query: a single freshly
# ingested point can fall outside the instant-query staleness window, but a
# range selector reliably returns it once ingestion has flushed.
got=""
for i in $(seq 1 15); do
  resp="$(curl -fsS --data-urlencode 'query=qw_smoke_metric[5m]' "$BASE/api/v1/query" 2>/dev/null || true)"
  # response is JSON: ...,"values":[[<ts>,"123"]]...
  if printf '%s' "$resp" | grep -q '"123"'; then
    got="123"; break
  fi
  sleep 1
done
[ "$got" = "123" ] || { echo "query did not return the written sample; last resp: ${resp:-<none>}"; docker logs "$NAME"; exit 1; }

# must run as the nonroot victoriametrics user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "version:"; docker exec "$NAME" /usr/bin/victoria-metrics -version
echo "smoke test passed (nonroot user: $user, read-only rootfs, write+query OK)"
