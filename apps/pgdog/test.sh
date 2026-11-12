#!/usr/bin/env bash
# Smoke test for a built PgDog image. Usage: test.sh <image-ref> [version]
# Starts our PostgreSQL image and PgDog in front of it (transaction pooling), then runs
# queries through PgDog with psql from the PostgreSQL image, and reads PgDog's admin database.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
PG_IMAGE=ghcr.io/quenchworks/images/postgresql:18.6
NET="quench-pgdog-$$"; PG="quench-pgdog-pg-$$"; DOG="quench-pgdog-smoke-$$"
WORK="$(mktemp -d)"
cleanup() { docker rm -f "$PG" "$DOG" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; rm -rf "$WORK"; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
ver="$(docker run --rm "$IMAGE" --version)"
echo "pgdog: $ver"
[ -z "$WANT" ] || grep -qF "v$WANT" <<<"$ver" || { echo "expected v$WANT"; exit 1; }

docker network create "$NET" >/dev/null
docker run -d --name "$PG" --network "$NET" -e POSTGRES_PASSWORD=quench -e POSTGRES_DB=app "$PG_IMAGE" >/dev/null
for i in $(seq 1 60); do
  docker exec -e PGPASSWORD=quench "$PG" psql -h /var/run/postgresql -U postgres -d app -qtAc "select 1" >/dev/null 2>&1 && break
  [ "$i" = 60 ] && { echo "postgres did not come up"; docker logs "$PG"; exit 1; }
  sleep 1
done

cat > "$WORK/pgdog.toml" <<TOML
[general]
host = "0.0.0.0"
port = 6432

[[databases]]
name = "app"
host = "$PG"
port = 5432

[admin]
password = "quench-admin"
TOML
cat > "$WORK/users.toml" <<TOML
[[users]]
name = "postgres"
database = "app"
password = "quench"
TOML
chmod 0644 "$WORK"/*.toml
docker create --name "$DOG" --network "$NET" "$IMAGE" >/dev/null
docker cp "$WORK/pgdog.toml" "$DOG:/etc/pgdog/pgdog.toml"
docker cp "$WORK/users.toml" "$DOG:/etc/pgdog/users.toml"
docker start "$DOG" >/dev/null

q() { docker exec -e PGPASSWORD="$1" "$PG" psql -h "$DOG" -p 6432 -U "$2" -d "$3" -qtAc "$4"; }
for i in $(seq 1 30); do
  out="$(q quench postgres app 'select 40 + 2' 2>/dev/null || true)"
  [ "$out" = 42 ] && break
  [ "$i" = 30 ] && { echo "query through pgdog failed: $out"; docker logs "$DOG" | tail -30; exit 1; }
  sleep 1
done
echo "query through pgdog: $out"
q quench postgres app 'create table t (n int); insert into t values (1), (2), (3)'
[ "$(q quench postgres app 'select sum(n) from t')" = 6 ] || { echo "write/read through pgdog failed"; exit 1; }
q quench-admin admin admin 'SHOW POOLS' | grep -q app || { echo "admin SHOW POOLS has no app pool"; docker logs "$DOG" | tail -20; exit 1; }
if q wrong postgres app 'select 1' >/dev/null 2>&1; then echo "a wrong password was accepted"; exit 1; fi

echo "smoke test passed ($ver, nonroot user: $user)"
