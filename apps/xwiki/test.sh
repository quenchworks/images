#!/usr/bin/env bash
# Smoke test for a built XWiki image. Usage: test.sh <image-ref> [version]
# Starts it against the QuenchWorks PostgreSQL image on a read-only rootfs, then
# requires the REST API root to answer 200, report the expected version, and the
# Hibernate store to have created its tables in PostgreSQL.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
TAG="quench-xwiki-smoke-$$"
NET="$TAG-net"
PG=ghcr.io/quenchworks/images/postgresql@sha256:4c0580fd2968b8d6e8fb008e0eb74b55c54464b041c235842e4d1cc37ed9a445
PW="pg-$$-secret"

cleanup() { docker rm -f "$TAG-pg" "$TAG-app" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

docker network create "$NET" >/dev/null
# (the QuenchWorks postgresql image creates POSTGRES_DB only when it differs from the superuser name)
docker run -d --name "$TAG-pg" --network "$NET" --network-alias pg \
  -e POSTGRES_PASSWORD="$PW" -e POSTGRES_DB=xwiki "$PG" >/dev/null
for i in $(seq 1 60); do
  docker exec "$TAG-pg" pg_isready -h 127.0.0.1 -U postgres -d xwiki >/dev/null 2>&1 && break
  [ "$i" = 60 ] && { echo "postgres never ready"; docker logs "$TAG-pg" | tail -20; exit 1; }
  sleep 1
done

docker run -d --name "$TAG-app" --network "$NET" --read-only --tmpfs /tmp \
  --tmpfs /var/lib/xwiki:uid=1001,gid=1001 -p 127.0.0.1:18080:8080 \
  -e DB_HOST=pg -e DB_DATABASE=xwiki -e DB_USER=postgres -e DB_PASSWORD="$PW" "$IMAGE" >/dev/null

# XWiki initializes on the first request and creates its schema; allow several minutes.
body=""
for i in $(seq 1 120); do
  code="$(curl -s -o /tmp/$TAG.rest -w '%{http_code}' http://127.0.0.1:18080/xwiki/rest || true)"
  [ "$code" = 200 ] && { body="$(cat /tmp/$TAG.rest)"; break; }
  [ "$i" = 120 ] && { echo "REST root never answered 200 (last $code)"; docker logs "$TAG-app" 2>&1 | tail -60; exit 1; }
  sleep 3
done
rm -f "/tmp/$TAG.rest"
echo "  /xwiki/rest: 200"
ver="$(sed -n 's|.*<version>\([^<]*\)</version>.*|\1|p' <<<"$body" | head -1)"
echo "reported version: $ver"
[ -n "$ver" ] || { echo "no version in REST root: ${body:0:300}"; exit 1; }
[ -z "$WANT" ] || [ "$ver" = "$WANT" ] || { echo "expected $WANT"; exit 1; }

tables="$(docker exec "$TAG-pg" psql -U postgres -d xwiki -tAc "select count(*) from information_schema.tables where table_name='xwikidoc'")"
[ "$tables" = 1 ] || { echo "xwikidoc table missing in PostgreSQL"; docker logs "$TAG-app" 2>&1 | tail -40; exit 1; }
echo "  PostgreSQL: xwikidoc table created"

echo "smoke test passed (version $ver, nonroot user: $user, PostgreSQL store, REST 200)"
