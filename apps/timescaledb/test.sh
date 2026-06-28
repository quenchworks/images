#!/usr/bin/env bash
# Smoke test for a built PostgreSQL+TimescaleDB image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-timescaledb-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE"
docker run -d --name "$NAME" -e POSTGRES_PASSWORD=quench -p 127.0.0.1:5432:5432 "$IMAGE" >/dev/null

# first boot runs initdb + CREATE EXTENSION timescaledb CASCADE, then restarts; wait it out.
# Connect over TCP (the server's socket lives in /var/run/postgresql, not the client default).
for i in $(seq 1 60); do
  if docker exec "$NAME" pg_isready -h 127.0.0.1 -q 2>/dev/null; then
    break
  fi
  [ "$i" = 60 ] && { echo "postgres did not become ready"; docker logs "$NAME" | tail -40; exit 1; }
  sleep 2
done

run() { docker exec -e PGPASSWORD=quench "$NAME" psql -h 127.0.0.1 -U postgres -d postgres -tAc "$1"; }

echo "checking timescaledb extension is installed"
exts="$(run "SELECT extname FROM pg_extension ORDER BY extname;")"
echo "$exts" | tr '\n' ' '; echo
echo "$exts" | grep -qx "timescaledb" || { echo "missing extension: timescaledb"; exit 1; }

echo "timescaledb version:"
ver="$(run "SELECT extversion FROM pg_extension WHERE extname='timescaledb';")"
echo "  $ver"
[ -n "$ver" ] || { echo "could not read timescaledb extversion"; exit 1; }

echo "exercising the TimescaleDB API (create a hypertable + insert)"
run "CREATE TABLE conditions (time TIMESTAMPTZ NOT NULL, device TEXT, temp DOUBLE PRECISION);" >/dev/null
run "SELECT create_hypertable('conditions','time');" >/dev/null
run "INSERT INTO conditions VALUES (now(),'dev1',21.5);" >/dev/null
got="$(run "SELECT count(*) FROM conditions;")"
echo "  rows in hypertable: ${got:-<none>}"
[ "$got" = "1" ] || { echo "hypertable insert/read failed"; exit 1; }

# must run as the nonroot postgres user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "version:"; run "SHOW server_version;" | head -1
echo "smoke test passed (nonroot user: $user)"
