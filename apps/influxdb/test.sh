#!/usr/bin/env bash
# Smoke test for a built InfluxDB image on a READ-ONLY rootfs. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-influxdb-smoke-$$"
ORG="quench"
BUCKET="smoke"
TOKEN="smoke-admin-token-0123456789"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE (read-only rootfs + writable tmpfs mounts)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /tmp:rw,mode=1777 \
  --tmpfs /var/lib/influxdb2:rw,mode=0700,uid=1001,gid=1001 \
  --tmpfs /etc/influxdb2:rw,mode=0700,uid=1001,gid=1001 \
  -e DOCKER_INFLUXDB_INIT_MODE=setup \
  -e DOCKER_INFLUXDB_INIT_USERNAME=admin \
  -e DOCKER_INFLUXDB_INIT_PASSWORD=quenchworks-smoke-pw \
  -e DOCKER_INFLUXDB_INIT_ORG="$ORG" \
  -e DOCKER_INFLUXDB_INIT_BUCKET="$BUCKET" \
  -e DOCKER_INFLUXDB_INIT_ADMIN_TOKEN="$TOKEN" \
  -p 127.0.0.1:8086:8086 "$IMAGE" >/dev/null

# Wait for the HTTP API health endpoint to report pass (setup runs first, then influxd).
ok=""
for i in $(seq 1 90); do
  if curl -fsS http://127.0.0.1:8086/health 2>/dev/null | grep -q '"status":"pass"'; then
    ok=1; break
  fi
  [ "$i" = 90 ] && { echo "influxd did not become healthy"; docker logs "$NAME" | tail -60; exit 1; }
  sleep 2
done
echo "/health: $(curl -fsS http://127.0.0.1:8086/health)"

echo "checking /ping returns 204"
code="$(curl -fsS -o /dev/null -w '%{http_code}' http://127.0.0.1:8086/ping)"
[ "$code" = "204" ] || { echo "expected 204 from /ping, got $code"; exit 1; }

echo "writing a point via /api/v2/write"
curl -fsS -XPOST "http://127.0.0.1:8086/api/v2/write?org=${ORG}&bucket=${BUCKET}&precision=s" \
  -H "Authorization: Token ${TOKEN}" \
  --data-binary "smoke,host=quench value=42 $(date +%s)" >/dev/null

echo "querying the point back via Flux /api/v2/query"
result="$(curl -fsS -XPOST "http://127.0.0.1:8086/api/v2/query?org=${ORG}" \
  -H "Authorization: Token ${TOKEN}" \
  -H 'Content-Type: application/vnd.flux' \
  --data "from(bucket:\"${BUCKET}\") |> range(start:-1h) |> filter(fn:(r)=>r._measurement==\"smoke\")")"
echo "$result" | grep -q ",42" || { echo "point not returned by query"; echo "$result"; exit 1; }

# must run as the nonroot influxdb user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user; write+query OK)"
