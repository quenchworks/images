#!/usr/bin/env bash
# Smoke test for a built mariadb image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-mariadb-smoke-$$"
PW="qw-smoke-pass"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE"
docker run -d --name "$NAME" \
  -e MARIADB_ROOT_PASSWORD="$PW" \
  -e MARIADB_DATABASE="qwapp" \
  "$IMAGE" >/dev/null

# wait for the server to accept connections over the socket
for i in $(seq 1 60); do
  if docker exec "$NAME" mariadb-admin --socket=/run/mysqld/mysqld.sock -uroot -p"$PW" ping 2>/dev/null | grep -q "mysqld is alive"; then
    break
  fi
  [ "$i" = 60 ] && { echo "mariadb did not become ready"; docker logs "$NAME" | tail -30; exit 1; }
  sleep 1
done

echo "alive; checking DDL/DML on the created database"
docker exec "$NAME" mariadb --socket=/run/mysqld/mysqld.sock -uroot -p"$PW" qwapp -e \
  "create table t(id int primary key, v varchar(32)); insert into t values (1,'hello'); select v from t where id=1;" \
  | grep -q hello || { echo "DDL/DML failed"; docker logs "$NAME" | tail -30; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "version:"; docker exec "$NAME" mariadbd --version
echo "smoke test passed (nonroot user: $user)"
