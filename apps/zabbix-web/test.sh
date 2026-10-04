#!/usr/bin/env bash
# Smoke test for the Zabbix web image. Usage: test.sh <image-ref> <version>
#   1. A real PostgreSQL (the catalog image) gets the schema from the published
#      zabbix-server image of the same version.
#   2. The frontend starts against it as nonroot on a read-only rootfs.
#   3. The JSON-RPC API logs in as the default Admin and reports the expected version,
#      which proves nginx, php-fpm, the pgsql extension and the env-driven config.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> <version>}"
WANT="${2:?usage: test.sh <image-ref> <version>}"
PG_IMAGE="ghcr.io/quenchworks/images/postgresql:18.6"
SERVER_IMAGE="ghcr.io/quenchworks/images/zabbix-server:$WANT"
NET="zbxw-smoke-$$"; PG="zbxw-pg-$$"; WEB="zbxw-web-$$"; PW="smoke-$$"; PORT=18080
TMP="$(mktemp -d)"
cleanup() { docker rm -f "$WEB" "$PG" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; rm -rf "$TMP"; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

docker network create "$NET" >/dev/null
docker run -d --name "$PG" --network "$NET" -e POSTGRES_PASSWORD="$PW" -e POSTGRES_DB=zabbix "$PG_IMAGE" >/dev/null
for i in $(seq 1 60); do
  out="$(docker exec -e PGPASSWORD="$PW" "$PG" psql -h /var/run/postgresql -U postgres -d zabbix -qtAc "select 1" 2>/dev/null || true)"
  [ "$out" = "1" ] && break
  [ "$i" = 60 ] && { echo "postgres did not become ready"; docker logs "$PG"; exit 1; }
  sleep 1
done
cid="$(docker create "$SERVER_IMAGE")"
docker cp "$cid:/usr/share/zabbix/database/postgresql/." "$TMP/" >/dev/null
docker rm "$cid" >/dev/null
for f in schema images data; do
  docker exec -i -e PGPASSWORD="$PW" "$PG" psql -h /var/run/postgresql -U postgres -d zabbix -q -v ON_ERROR_STOP=1 <"$TMP/$f.sql" >/dev/null
done

docker run -d --name "$WEB" --network "$NET" --read-only --tmpfs /tmp:uid=1001,gid=1001,mode=1777 \
  -e ZBX_DB_HOST="$PG" -e ZBX_DB_USER=postgres -e ZBX_DB_PASSWORD="$PW" \
  -p "127.0.0.1:$PORT:8080" "$IMAGE" >/dev/null
api() { curl -fsS -H 'Content-Type: application/json-rpc' -d "$1" "http://127.0.0.1:$PORT/api_jsonrpc.php"; }
for i in $(seq 1 60); do
  ver="$(api '{"jsonrpc":"2.0","method":"apiinfo.version","params":[],"id":1}' 2>/dev/null || true)"
  grep -q '"result"' <<<"$ver" && break
  [ "$i" = 60 ] && { echo "API never answered: $ver"; docker logs "$WEB" 2>&1 | tail -40; exit 1; }
  sleep 1
done
echo "apiinfo.version: $ver"
grep -q "\"result\":\"$WANT\"" <<<"$ver" || { echo "expected version $WANT"; exit 1; }
login="$(api '{"jsonrpc":"2.0","method":"user.login","params":{"username":"Admin","password":"zabbix"},"id":2}')"
grep -qE '"result":"[0-9a-f]{32}' <<<"$login" || { echo "login failed: $login"; docker logs "$WEB" 2>&1 | tail -40; exit 1; }
echo "smoke test passed (zabbix-web $WANT: API login as Admin on PostgreSQL 18, nonroot 1001)"
