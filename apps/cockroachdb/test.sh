#!/usr/bin/env bash
# Smoke test for a built CockroachDB image. Usage: test.sh <image-ref>
# Runs with a READ-ONLY rootfs and a writable tmpfs (exec) for the store dir, waits for
# the HTTP /health?ready=1 to return 200, then exercises a SQL roundtrip (CREATE DATABASE
# / CREATE TABLE / INSERT / SELECT) via the in-image `cockroach sql` client. Confirms
# nonroot uid 1001.
#
# LICENSE: CockroachDB is BSL-1.1 — NOT OSI open source; converts to Apache-2.0 after 3y.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-cockroachdb-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE (read-only rootfs, writable tmpfs store)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /cockroach/cockroach-data:rw,mode=1777,exec \
  -p 18080:8080 \
  -p 26257:26257 \
  "$IMAGE" >/dev/null

# confirm nonroot uid 1001
UID_OUT="$(docker exec "$NAME" id -u)"
echo "runtime uid: $UID_OUT"
[ "$UID_OUT" = "1001" ] || { echo "FAIL: not running as uid 1001"; exit 1; }

# wait for HTTP /health?ready=1 -> 200
echo "waiting for /health?ready=1"
ok=""
for i in $(seq 1 60); do
  code="$(curl -s -o /dev/null -w '%{http_code}' "http://localhost:18080/health?ready=1" 2>/dev/null || echo 000)"
  if [ "$code" = "200" ]; then
    ok=1; echo "health ready (200) after ${i}s"; break
  fi
  if [ "$i" = 60 ]; then
    echo "FAIL: cockroach did not become ready (last code: $code)"; docker logs "$NAME" 2>&1 | tail -80; exit 1
  fi
  sleep 1
done

# SQL roundtrip via the in-image cockroach SQL client
echo "SQL CREATE / INSERT / SELECT roundtrip (in-image cockroach sql):"
OUT="$(docker exec "$NAME" cockroach sql --insecure --host=localhost:26257 --format=tsv -e "
CREATE DATABASE IF NOT EXISTS gate;
CREATE TABLE IF NOT EXISTS gate.t (id INT PRIMARY KEY, v STRING);
INSERT INTO gate.t VALUES (1, 'quench') ON CONFLICT (id) DO UPDATE SET v = excluded.v;
SELECT v FROM gate.t WHERE id = 1;
")"
echo "$OUT"
echo "$OUT" | grep -qx "quench" || { echo "FAIL: expected 'quench' from SQL roundtrip"; docker logs "$NAME" 2>&1 | tail -40; exit 1; }
echo "  SQL roundtrip returned 'quench'"

echo "PASS: cockroachdb smoke test green"
