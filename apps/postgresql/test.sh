#!/usr/bin/env bash
# Smoke test for a built postgresql image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-postgresql-smoke-$$"
PW="qw-smoke-pass"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE"
# initdb bootstraps on first boot; create an app db to exercise that path too
docker run -d --name "$NAME" \
  -e POSTGRES_PASSWORD="$PW" \
  -e POSTGRES_DB="qwapp" \
  "$IMAGE" >/dev/null

# wait for the server to accept connections
for i in $(seq 1 60); do
  if docker exec "$NAME" pg_isready -h /var/run/postgresql -q 2>/dev/null; then
    break
  fi
  [ "$i" = 60 ] && { echo "postgres did not become ready"; docker logs "$NAME"; exit 1; }
  sleep 1
done

echo "pg_isready ok; checking auth + DDL/DML on the created database"
docker exec -e PGPASSWORD="$PW" "$NAME" \
  psql -h /var/run/postgresql -U postgres -d qwapp -v ON_ERROR_STOP=1 -c \
  "create table t(id int primary key, v text); insert into t values (1,'hello'); select v from t where id=1;" \
  | grep -q hello || { echo "DDL/DML failed"; docker logs "$NAME"; exit 1; }

# must run as the nonroot postgres user (uid 1001). The image has busybox but we
# check the configured user rather than relying on runtime state.
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "server version:"
docker exec "$NAME" postgres --version

echo "smoke test passed (nonroot user: $user)"
