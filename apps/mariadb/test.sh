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

# MariaDB Operator runs the image as uid 999 with its config in conf.d and
# extra mariadbd flags as arguments: run it that way and check both land.
NAME2="$NAME-op"; CONF="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
trap 'cleanup; docker rm -f "$NAME2" >/dev/null 2>&1 || true; rm -rf "$CONF"' EXIT
printf '[mariadbd]\nmax_connections=123\n' > "$CONF/quench.cnf"; chmod -R a+rX "$CONF"
docker run -d --name "$NAME2" --user 999:999 --tmpfs /var/lib/mysql:uid=999,gid=999 -e MARIADB_ROOT_PASSWORD="$PW" \
  -v "$CONF:/etc/mysql/conf.d:ro" "$IMAGE" mariadbd --max-allowed-packet=33554432 >/dev/null
for i in $(seq 1 60); do
  docker exec "$NAME2" mariadb-admin --socket=/run/mysqld/mysqld.sock -uroot -p"$PW" ping 2>/dev/null | grep -q "mysqld is alive" && break
  [ "$i" = 60 ] && { echo "mariadb as uid 999 did not become ready"; docker logs "$NAME2" | tail -30; exit 1; }
  sleep 1
done
got="$(docker exec "$NAME2" mariadb --socket=/run/mysqld/mysqld.sock -uroot -p"$PW" -N -e 'SELECT @@max_connections, @@max_allowed_packet')"
echo "uid 999, conf.d and args: $got"
[ "$got" = "$(printf '123\t33554432')" ] || { echo "conf.d or args were not applied"; exit 1; }

echo "version:"; docker exec "$NAME" mariadbd --version
echo "smoke test passed (nonroot user: $user)"
