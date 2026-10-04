#!/usr/bin/env bash
# Smoke test for the Zabbix server image. Usage: test.sh <image-ref> [version]
#   1. zabbix_server -V reports the expected version.
#   2. A real PostgreSQL (the catalog image) gets the schema shipped in this image.
#   3. The server starts against it as nonroot on a read-only rootfs, and its log shows
#      the server processes started and the database connection made.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
PG_IMAGE="ghcr.io/quenchworks/images/postgresql:18.6"
NET="zbx-smoke-$$"; PG="zbx-pg-$$"; ZS="zbx-server-$$"; PW="smoke-$$"; VOL="zbx-conf-$$"
TMP="$(mktemp -d)"
cleanup() { docker rm -f "$ZS" "$PG" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; docker volume rm "$VOL" >/dev/null 2>&1 || true; rm -rf "$TMP"; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

ver="$(docker run --rm --entrypoint /usr/bin/zabbix_server "$IMAGE" -V 2>&1 | head -1)"
echo "$ver"
grep -q "zabbix_server (Zabbix)" <<<"$ver" || { echo "zabbix_server -V failed"; exit 1; }
[ -z "$WANT" ] || grep -q "(Zabbix) $WANT" <<<"$ver" || { echo "expected version $WANT"; exit 1; }

docker network create "$NET" >/dev/null
docker run -d --name "$PG" --network "$NET" -e POSTGRES_PASSWORD="$PW" -e POSTGRES_DB=zabbix "$PG_IMAGE" >/dev/null
for i in $(seq 1 60); do
  out="$(docker exec -e PGPASSWORD="$PW" "$PG" psql -h /var/run/postgresql -U postgres -d zabbix -qtAc "select 1" 2>/dev/null || true)"
  [ "$out" = "1" ] && break
  [ "$i" = 60 ] && { echo "postgres did not become ready"; docker logs "$PG"; exit 1; }
  sleep 1
done

# the schema comes out of the image under test, not from anywhere else
cid="$(docker create "$IMAGE")"
docker cp "$cid:/usr/share/zabbix/database/postgresql/." "$TMP/" >/dev/null
docker rm "$cid" >/dev/null
for f in schema images data; do
  docker exec -i -e PGPASSWORD="$PW" "$PG" psql -h /var/run/postgresql -U postgres -d zabbix -q -v ON_ERROR_STOP=1 <"$TMP/$f.sql" >/dev/null
done

# A named volume, not a bind mount: some Docker hosts cannot share the host's /tmp. The
# postgres image has a shell, so it writes the include file the chart would mount.
docker volume create "$VOL" >/dev/null
docker run --rm --user 0 -v "$VOL:/c" --entrypoint /bin/sh "$PG_IMAGE" -c \
  "printf 'DBHost=$PG\nDBUser=postgres\nDBPassword=$PW\n' >/c/db.conf && chmod 644 /c/db.conf"
docker run -d --name "$ZS" --network "$NET" --read-only --tmpfs /tmp:uid=1001,gid=1001 \
  -v "$VOL:/etc/zabbix/zabbix_server.d:ro" "$IMAGE" >/dev/null
for i in $(seq 1 60); do
  logs="$(docker logs "$ZS" 2>&1)"
  grep -q 'server #0 started \[main process\]' <<<"$logs" && grep -q 'started \[trapper #1\]' <<<"$logs" && break
  [ "$(docker inspect -f '{{.State.Running}}' "$ZS")" = true ] || { echo "server exited:"; tail -40 <<<"$logs"; exit 1; }
  [ "$i" = 60 ] && { echo "server processes never started:"; tail -40 <<<"$logs"; exit 1; }
  sleep 1
done
grep -qi 'database is down\|cannot connect' <<<"$logs" && { echo "database errors:"; tail -40 <<<"$logs"; exit 1; }
echo "smoke test passed ($ver; schema loaded, server and trapper started on PostgreSQL 18)"
