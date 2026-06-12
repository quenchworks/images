#!/usr/bin/env bash
# ============================================================================
# LICENSE: MongoDB Community Server is SSPL-1.0 — *** NOT OSI-APPROVED ***.
# The OSI explicitly declined the SSPL; it is NOT open source. Caution tier.
# Clean truly-open alternative: QuenchWorks FerretDB + standalone documentdb
# (MongoDB-wire compatible, Apache-2.0/PostgreSQL-licensed) — prefer them.
# ============================================================================
#
# Smoke test for a built MongoDB image. Usage: test.sh <image-ref>
# Runs with a READ-ONLY rootfs + writable tmpfs (exec) for /data, boots with a
# root user (auth bootstrap), waits for it to come up, then connects as root via
# the in-image mongosh and exercises insertOne/findOne + ping. Confirms uid 1001.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-mongodb-smoke-$$"
ROOT_USER="root"
ROOT_PASS="quenchtest"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE (read-only rootfs, writable tmpfs /data)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /data:rw,mode=1777,exec \
  -p 27018:27017 \
  -e MONGO_INITDB_ROOT_USERNAME="$ROOT_USER" \
  -e MONGO_INITDB_ROOT_PASSWORD="$ROOT_PASS" \
  -e MONGO_INITDB_DATABASE="quench" \
  "$IMAGE" >/dev/null

# confirm nonroot uid 1001
UID_OUT="$(docker exec "$NAME" id -u)"
echo "runtime uid: $UID_OUT"
[ "$UID_OUT" = "1001" ] || { echo "FAIL: not running as uid 1001"; exit 1; }

URI="mongodb://${ROOT_USER}:${ROOT_PASS}@localhost:27018/admin?authSource=admin"

# in-image mongosh writes config under $HOME; the rootfs is read-only so point it
# at the writable /data/log (same as the entrypoint does) to avoid a noisy warning.
EXEC="docker exec -e HOME=/data/log $NAME mongosh --quiet"

# wait for mongod to accept authenticated connections (auth bootstrap ~10-25s)
echo "waiting for authenticated ping"
ok=""
for i in $(seq 1 90); do
  if $EXEC "mongodb://${ROOT_USER}:${ROOT_PASS}@localhost:27017/admin?authSource=admin" \
       --eval 'db.runCommand({ping:1}).ok' 2>/dev/null | grep -q 1; then
    ok=1; echo "authenticated ping ok after ${i}s"; break
  fi
  if [ "$i" = 90 ]; then
    echo "FAIL: mongod never accepted authenticated connection"
    docker logs "$NAME" 2>&1 | tail -80; exit 1
  fi
  sleep 1
done

m() {
  $EXEC "mongodb://${ROOT_USER}:${ROOT_PASS}@localhost:27017/quench?authSource=admin" \
    --eval "$1"
}

echo "server version:"
VER="$(m 'db.version()')"
echo "  -> $VER"

echo "insertOne / findOne roundtrip:"
m 'db.gate.insertOne({k:"quench"})' >/dev/null
FOUND="$(m 'db.gate.findOne().k')"
echo "  findOne().k -> $FOUND"
[ "$FOUND" = "quench" ] || { echo "FAIL: expected 'quench', got '$FOUND'"; exit 1; }

echo "ping command:"
PING="$(m 'db.runCommand({ping:1}).ok')"
echo "  ping ok -> $PING"
[ "$PING" = "1" ] || { echo "FAIL: ping not ok"; exit 1; }

# verify auth is actually enforced (no creds must be rejected)
echo "auth enforcement check (no creds must fail):"
if $EXEC "mongodb://localhost:27017/admin" \
     --eval 'db.adminCommand({listDatabases:1})' >/dev/null 2>&1; then
  echo "FAIL: unauthenticated admin command succeeded"; exit 1
fi
echo "  unauthenticated command correctly rejected"

echo "PASS: mongodb smoke test green"
