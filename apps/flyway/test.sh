#!/usr/bin/env bash
# Smoke test for a built flyway image. Usage: test.sh <image-ref> [version]
# Real migrations: apply two versioned scripts to a live PostgreSQL (the QuenchWorks
# postgresql image) from a read-only root, then require `info` to report both applied
# and PostgreSQL to show the migrated table.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NET="flyway-smoke-$$"; PG="$NET-pg"; PW="quench-smoke-pw"
cleanup() { docker rm -f "$PG" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

ver="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" -v 2>&1 || true)"
echo "$ver" | grep -m1 -i 'flyway'
[ -z "$WANT" ] || echo "$ver" | grep -q "$WANT" || { echo "expected version $WANT"; echo "$ver" | tail -5; exit 1; }

docker network create "$NET" >/dev/null
docker run -d --name "$PG" --network "$NET" -e POSTGRES_PASSWORD="$PW" ghcr.io/quenchworks/images/postgresql:18.6 >/dev/null
for i in $(seq 1 60); do
  docker exec -e PGPASSWORD="$PW" "$PG" psql -h 127.0.0.1 -U postgres -tAc 'select 1' >/dev/null 2>&1 && break
  [ "$i" = 60 ] && { echo "postgres did not become ready"; docker logs "$PG" 2>&1 | tail -20; exit 1; }
  sleep 2
done

WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
trap 'rm -rf "$WORK"; cleanup' EXIT
printf 'create table orders (id int primary key);\n' > "$WORK/V1__orders.sql"
printf 'alter table orders add column note text;\n' > "$WORK/V2__note.sql"
chmod -R a+rX "$WORK"
fw() { docker run --rm --network "$NET" --read-only --tmpfs /tmp --tmpfs /home/nonroot:uid=1001,gid=1001 \
  -v "$WORK:/flyway/sql:ro" "$IMAGE" -url="jdbc:postgresql://$PG:5432/postgres" -user=postgres -password="$PW" \
  -locations=filesystem:/flyway/sql "$@"; }
fw migrate | tee "$WORK/migrate.log"
grep -q 'Successfully applied 2 migrations' "$WORK/migrate.log" || { echo "migrations not applied"; exit 1; }
fw info | tee "$WORK/info.log"
[ "$(grep -c 'Success' "$WORK/info.log")" -ge 2 ] || { echo "info does not list 2 successful migrations"; exit 1; }
cols="$(docker exec -e PGPASSWORD="$PW" "$PG" psql -h 127.0.0.1 -U postgres -tAc "select string_agg(column_name, ',' order by column_name) from information_schema.columns where table_name='orders'")"
[ "$cols" = "id,note" ] || { echo "orders columns are '$cols'"; exit 1; }
echo "smoke test passed (flyway ${WANT:-?}, uid $user, 2 migrations applied to PostgreSQL)"
