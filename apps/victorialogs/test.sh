#!/usr/bin/env bash
# Smoke test for a built VictoriaLogs image. Usage: test.sh <image-ref>
# Runs read-only rootfs + writable tmpfs for the storage dir (the read-only
# posture the chart ships), waits for /health, ingests a log line via the
# jsonline API, then queries it back with LogsQL and asserts the message.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-victorialogs-smoke-$$"
BASE="http://127.0.0.1:9428"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE (read-only rootfs, tmpfs /victoria-logs-data)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /victoria-logs-data:rw,mode=1777 \
  -p 127.0.0.1:9428:9428 \
  "$IMAGE" >/dev/null

# wait for the HTTP API to report healthy
for i in $(seq 1 30); do
  if [ "$(curl -fsS "$BASE/health" 2>/dev/null || true)" = "OK" ]; then
    break
  fi
  [ "$i" = 30 ] && { echo "victoria-logs did not become healthy"; docker logs "$NAME"; exit 1; }
  sleep 1
done

echo "healthy; ingesting a log line via the jsonline API"
# jsonline is newline-delimited JSON and requires the stream+json content type;
# without it the body is treated as a form and silently ingests 0 rows.
curl -fsS -X POST "$BASE/insert/jsonline" \
  -H 'Content-Type: application/stream+json' \
  --data-binary $'{"_msg":"qw_smoke_log","source":"smoke"}\n' >/dev/null

echo "querying it back with LogsQL"
# LogsQL requires a time filter; _time:1d scopes to the just-ingested record.
got=""
for i in $(seq 1 20); do
  resp="$(curl -fsS "$BASE/select/logsql/query" --data-urlencode 'query=_time:1d qw_smoke_log' 2>/dev/null || true)"
  if printf '%s' "$resp" | grep -q 'qw_smoke_log'; then
    got="qw_smoke_log"; break
  fi
  sleep 1
done
[ "$got" = "qw_smoke_log" ] || { echo "query did not return the ingested log; last resp: ${resp:-<none>}"; docker logs "$NAME"; exit 1; }

# must run as the nonroot victorialogs user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "version:"; docker exec "$NAME" /usr/bin/victoria-logs -version
echo "smoke test passed (nonroot user: $user, read-only rootfs, ingest+query OK)"
