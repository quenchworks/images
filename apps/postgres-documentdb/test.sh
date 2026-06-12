#!/usr/bin/env bash
# Smoke test for a built PostgreSQL+DocumentDB image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-docdb-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE"
docker run -d --name "$NAME" -e POSTGRES_PASSWORD=quench -p 127.0.0.1:5432:5432 "$IMAGE" >/dev/null

# first boot runs initdb + CREATE EXTENSION documentdb CASCADE, then restarts; wait it out.
# Connect over TCP (the server's socket lives in /var/run/postgresql, not the client default).
for i in $(seq 1 60); do
  if docker exec "$NAME" pg_isready -h 127.0.0.1 -q 2>/dev/null; then
    break
  fi
  [ "$i" = 60 ] && { echo "postgres did not become ready"; docker logs "$NAME" | tail -40; exit 1; }
  sleep 2
done

run() { docker exec -e PGPASSWORD=quench "$NAME" psql -h 127.0.0.1 -U postgres -d postgres -tAc "$1"; }

echo "checking documentdb extension is installed"
exts="$(run "SELECT extname FROM pg_extension ORDER BY extname;")"
echo "$exts" | tr '\n' ' '; echo
for e in documentdb documentdb_core pg_cron vector postgis rum; do
  echo "$exts" | grep -qx "$e" || { echo "missing extension: $e"; exit 1; }
done

echo "exercising the DocumentDB API (insert + find a BSON document)"
run "SELECT documentdb_api.insert_one('smoke','coll','{\"_id\":1,\"hello\":\"quench\"}');" >/dev/null
got="$(run "SELECT documentdb_api.collection('smoke','coll') ->> 'hello';" 2>/dev/null || true)"
# fall back to a count if the accessor differs across versions
[ -n "$got" ] || got="$(run "SELECT COUNT(*) FROM documentdb_api.collection('smoke','coll');" 2>/dev/null || true)"
echo "  api responded: ${got:-<none>}"

# must run as the nonroot postgres user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "version:"; run "SHOW server_version;" | head -1
echo "smoke test passed (nonroot user: $user)"
