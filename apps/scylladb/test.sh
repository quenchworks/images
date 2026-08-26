#!/usr/bin/env bash
# Smoke test for a built ScyllaDB image. Usage: test.sh <image-ref>
# Runs with a READ-ONLY rootfs + writable tmpfs mounts (exec, mode=1777 — Scylla's
# libreloc loader needs exec). Single-node developer-mode boot takes ~30-90s. Verifies:
#   - nonroot uid 1001
#   - REST API on 10000 returns the release version (GET /storage_service/scylla_release_version)
#   - CQL on 9042: CREATE KEYSPACE / TABLE, INSERT, SELECT roundtrip returning a value,
#     driven by a throwaway python cassandra-driver container on a shared docker network.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-scylladb-smoke-$$"
NET="quench-scylladb-net-$$"

cleanup() {
  docker rm -f "$NAME" >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker network create "$NET" >/dev/null

echo "starting $IMAGE (read-only rootfs, writable tmpfs mounts, smp=1 memory=1G developer-mode)"
docker run -d --name "$NAME" --network "$NET" \
  --read-only \
  --tmpfs /etc/scylla:rw,mode=1777,exec \
  --tmpfs /var/lib/scylla:rw,mode=1777,exec \
  --tmpfs /tmp:rw,mode=1777,exec \
  -p 19042:9042 -p 11000:10000 \
  "$IMAGE" >/dev/null

# confirm nonroot uid 1001
UID_OUT="$(docker exec "$NAME" id -u)"
echo "runtime uid: $UID_OUT"
[ "$UID_OUT" = "1001" ] || { echo "FAIL: not running as uid 1001"; docker logs "$NAME" 2>&1 | tail -40; exit 1; }

# wait for the REST API (10000) to answer the release-version endpoint
echo "waiting for the REST API on 10000 (scylla_release_version)"
VER=""
for i in $(seq 1 120); do
  VER="$(curl -fsS "http://localhost:11000/storage_service/scylla_release_version" 2>/dev/null || true)"
  if [ -n "$VER" ]; then echo "REST API up after ${i}s -> $VER"; break; fi
  if [ "$i" = 120 ]; then
    echo "FAIL: REST API never came up"; docker logs "$NAME" 2>&1 | tail -80; exit 1
  fi
  sleep 1
done

# wait for the CQL port to accept native transport (the server opens 9042 after the REST API)
echo "waiting for CQL native transport on 9042"
SCY_IP="$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$NAME")"
ready=""
for i in $(seq 1 120); do
  if docker run --rm --network "$NET" busybox sh -c "nc -z $SCY_IP 9042" >/dev/null 2>&1; then
    ready=1; echo "CQL port open after ${i}s"; break
  fi
  [ "$i" = 120 ] && { echo "FAIL: 9042 never opened"; docker logs "$NAME" 2>&1 | tail -80; exit 1; }
  sleep 1
done

# full CQL roundtrip via a throwaway python cassandra-driver container
echo "CQL roundtrip (CREATE KEYSPACE/TABLE, INSERT, SELECT) via cassandra-driver"
# Write the python driver script to a host temp file with a QUOTED heredoc delimiter so
# the CQL map literals ('class': ...) never collide with shell quoting, then feed it to
# the throwaway container's python on stdin.
PYFILE="$(mktemp)"
cat > "$PYFILE" <<'PY'
import time, sys, os
from cassandra.cluster import Cluster
from cassandra.policies import RoundRobinPolicy
ip = os.environ["SCY_IP"]
last = None
for _ in range(40):
    try:
        c = Cluster([ip], port=9042, protocol_version=4,
                    load_balancing_policy=RoundRobinPolicy())
        s = c.connect()
        break
    except Exception as e:
        last = e; time.sleep(3)
else:
    print("FAIL: could not connect to CQL:", last); sys.exit(1)
# NetworkTopologyStrategy, not SimpleStrategy: Scylla 2026.x creates keyspaces
# with tablets enabled by default and rejects SimpleStrategy outright
# ("SimpleStrategy doesn't support tablet replication").
s.execute("CREATE KEYSPACE IF NOT EXISTS smoke WITH replication = "
          "{'class': 'NetworkTopologyStrategy', 'replication_factor': 1}")
s.execute("CREATE TABLE IF NOT EXISTS smoke.t (id int PRIMARY KEY, name text)")
s.execute("INSERT INTO smoke.t (id, name) VALUES (1, %s)", ("alpha",))
s.execute("INSERT INTO smoke.t (id, name) VALUES (2, %s)", ("beta",))
rows = list(s.execute("SELECT name FROM smoke.t WHERE id = 2"))
val = rows[0].name if rows else None
print("select id=2 ->", val)
assert val == "beta", f"expected beta, got {val!r}"
cnt = list(s.execute("SELECT count(*) AS c FROM smoke.t"))[0].c
print("row count ->", cnt)
assert cnt == 2, f"expected 2 rows, got {cnt}"
print("CQL roundtrip OK")
PY
docker run --rm -i --network "$NET" -e SCY_IP="$SCY_IP" python:3.12-slim bash -c '
  set -e
  pip install --quiet --disable-pip-version-check "cassandra-driver==3.29.2" >/dev/null
  python -
' < "$PYFILE"
rm -f "$PYFILE"

echo "PASS: scylladb smoke test green (REST API $VER, CQL roundtrip ok)"
