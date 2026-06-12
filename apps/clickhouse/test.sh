#!/usr/bin/env bash
# Smoke test for a built ClickHouse image. Usage: test.sh <image-ref>
# Runs with a READ-ONLY rootfs and writable tmpfs mounts, waits for the HTTP /ping to
# return "Ok.", then exercises an authenticated query roundtrip (version + CREATE /
# INSERT / SELECT) over the HTTP interface (8123). Confirms nonroot uid 1001.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-clickhouse-smoke-$$"
CH_USER="smokeuser"
CH_PASS="smokepass"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE (read-only rootfs, writable tmpfs mounts)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /etc/clickhouse-server:rw,mode=1777 \
  --tmpfs /var/lib/clickhouse:rw,mode=1777,exec \
  --tmpfs /var/log/clickhouse-server:rw,mode=1777 \
  -p 18123:8123 \
  -e CLICKHOUSE_USER="$CH_USER" \
  -e CLICKHOUSE_PASSWORD="$CH_PASS" \
  -e CLICKHOUSE_DB="smoke" \
  "$IMAGE" >/dev/null

# confirm nonroot uid 1001
UID_OUT="$(docker exec "$NAME" id -u)"
echo "runtime uid: $UID_OUT"
[ "$UID_OUT" = "1001" ] || { echo "FAIL: not running as uid 1001"; exit 1; }

# wait for HTTP /ping -> Ok.
echo "waiting for /ping"
ok=""
for i in $(seq 1 60); do
  if curl -fsS "http://localhost:18123/ping" 2>/dev/null | grep -q "Ok."; then
    ok=1; echo "ping ok after ${i}s"; break
  fi
  if [ "$i" = 60 ]; then
    echo "FAIL: clickhouse did not answer /ping"; docker logs "$NAME" 2>&1 | tail -60; exit 1
  fi
  sleep 1
done

q() { curl -fsS -u "$CH_USER:$CH_PASS" "http://localhost:18123/" --data-binary "$1"; }

echo "SELECT version():"
VER="$(q 'SELECT version()')"
echo "  -> $VER"

echo "CREATE / INSERT / SELECT roundtrip:"
q 'CREATE TABLE IF NOT EXISTS smoke.t (id UInt32, name String) ENGINE = MergeTree ORDER BY id' >/dev/null
q "INSERT INTO smoke.t VALUES (1, 'alpha'), (2, 'beta')" >/dev/null
ROWS="$(q 'SELECT count() FROM smoke.t')"
echo "  row count -> $ROWS"
[ "$ROWS" = "2" ] || { echo "FAIL: expected 2 rows, got $ROWS"; exit 1; }
NAME_OUT="$(q "SELECT name FROM smoke.t WHERE id = 2")"
echo "  select id=2 -> $NAME_OUT"
[ "$NAME_OUT" = "beta" ] || { echo "FAIL: expected 'beta', got '$NAME_OUT'"; exit 1; }

# verify auth is actually enforced (wrong password must be rejected)
echo "auth enforcement check (wrong password must fail):"
if curl -fsS -u "$CH_USER:wrongpass" "http://localhost:18123/" --data-binary 'SELECT 1' >/dev/null 2>&1; then
  echo "FAIL: query succeeded with wrong password"; exit 1
fi
echo "  wrong password correctly rejected"

# in-image clickhouse client roundtrip
echo "in-image clickhouse client check:"
CLIENT_VER="$(docker exec "$NAME" clickhouse client --user "$CH_USER" --password "$CH_PASS" --query 'SELECT version()')"
echo "  client -> $CLIENT_VER"

echo "PASS: clickhouse smoke test green"
