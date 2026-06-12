#!/usr/bin/env bash
# Smoke test for a built Cassandra image. Usage: test.sh <image-ref>
# Runs with a READ-ONLY rootfs and writable tmpfs mounts, verifies the node reaches
# UN (Up/Normal) via in-image nodetool, then does a CQL roundtrip with a throwaway
# cqlsh client (the runtime image ships no cqlsh/python). Confirms nonroot uid 1001.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-cassandra-smoke-$$"
NET="quench-cass-net-$$"

cleanup() {
  docker rm -f "$NAME" >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker network create "$NET" >/dev/null

echo "starting $IMAGE (read-only rootfs, writable tmpfs mounts)"
docker run -d --name "$NAME" --network "$NET" \
  --read-only \
  --tmpfs /conf:rw,mode=1777 \
  --tmpfs /var/lib/cassandra:rw,mode=1777,exec \
  --tmpfs /var/log/cassandra:rw,mode=1777 \
  -e MAX_HEAP_SIZE=512M -e HEAP_NEWSIZE=128M \
  "$IMAGE" >/dev/null

# Cassandra + JVM bootstrap takes ~30-60s; poll nodetool status for the node to be UN.
ok=""
for i in $(seq 1 60); do
  if docker exec "$NAME" /opt/cassandra/bin/nodetool status 2>/dev/null | grep -qE '^UN '; then
    ok=1; break
  fi
  if [ "$i" = 60 ]; then
    echo "cassandra did not reach UN"; docker logs "$NAME" 2>&1 | tail -50; exit 1
  fi
  sleep 3
done
echo "node up:"; docker exec "$NAME" /opt/cassandra/bin/nodetool status 2>/dev/null | grep -E '^UN ' | head -1

IP="$(docker inspect "$NAME" --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}')"
echo "node IP: $IP"

# CQL roundtrip via a throwaway cqlsh from the same image (it has bin/cqlsh + pylib,
# but no python at runtime -- so run cqlsh from a stock python image with the driver).
echo "CQL roundtrip via throwaway cassandra-driver client"
docker run --rm --network "$NET" python:3.12-slim bash -lc "
  pip install --quiet cassandra-driver >/dev/null 2>&1
  python - <<PY
from cassandra.cluster import Cluster
import time, sys
for _ in range(30):
    try:
        c = Cluster(['$IP'], port=9042); s = c.connect(); break
    except Exception:
        time.sleep(2)
else:
    print('could not connect'); sys.exit(1)
s.execute(\"CREATE KEYSPACE IF NOT EXISTS smoke WITH replication = {'class':'SimpleStrategy','replication_factor':1}\")
s.execute('CREATE TABLE IF NOT EXISTS smoke.t (k text PRIMARY KEY, v text)')
s.execute(\"INSERT INTO smoke.t (k,v) VALUES ('quench','works')\")
row = s.execute(\"SELECT v FROM smoke.t WHERE k='quench'\").one()
assert row and row.v == 'works', 'roundtrip mismatch'
print('CQL roundtrip OK:', row.v)
PY
"

# must run as the nonroot cassandra user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
