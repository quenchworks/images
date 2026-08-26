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

# Wait until the REAL server accepts a connection to the app db. pg_isready is
# not enough: it also answers OK to initdb's temporary bootstrap server, which
# only creates qwapp at the end and then shuts down before the final server
# starts -- connecting in that window fails with "database qwapp does not
# exist" or "the database system is shutting down".
for i in $(seq 1 60); do
  out="$(docker exec -e PGPASSWORD="$PW" "$NAME" \
    psql -h /var/run/postgresql -U postgres -d qwapp -qtAc "select 1" 2>/dev/null || true)"
  [ "$out" = "1" ] && break
  [ "$i" = 60 ] && { echo "postgres did not become ready"; docker logs "$NAME"; exit 1; }
  sleep 1
done

echo "qwapp reachable; checking auth + DDL/DML on the created database"
out="$(docker exec -e PGPASSWORD="$PW" "$NAME" \
  psql -h /var/run/postgresql -U postgres -d qwapp -v ON_ERROR_STOP=1 -c \
  "create table t(id int primary key, v text); insert into t values (1,'hello'); select v from t where id=1;")" \
  || { echo "DDL/DML failed"; docker logs "$NAME"; exit 1; }
grep -q hello <<<"$out" || { echo "DDL/DML failed"; docker logs "$NAME"; exit 1; }

# must run as the nonroot postgres user (uid 1001). The image has busybox but we
# check the configured user rather than relying on runtime state.
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "server version:"
docker exec "$NAME" postgres --version

echo "smoke test passed (nonroot user: $user)"
