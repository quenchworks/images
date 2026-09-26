#!/usr/bin/env bash
# Smoke test for a built Hasura GraphQL Engine image. Usage: test.sh <image-ref> [version]
# Starts it against the QuenchWorks PostgreSQL image with an admin secret, then
# exercises the real path: create a table with run_sql, track it through the
# metadata API, insert and read a row over GraphQL, and require an anonymous
# request to be refused.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
TAG="quench-hasura-smoke-$$"
NET="$TAG-net"
PG=ghcr.io/quenchworks/images/postgresql@sha256:4c0580fd2968b8d6e8fb008e0eb74b55c54464b041c235842e4d1cc37ed9a445
PW="pg-$$-secret"
SECRET="admin-$$-secret"
URL=http://127.0.0.1:18080

cleanup() { docker rm -f "$TAG-pg" "$TAG-hge" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
ver="$(docker run --rm --entrypoint /usr/bin/graphql-engine "$IMAGE" version)"
echo "$ver"
grep -q -- "-ce$" <<<"$ver" || { echo "not the Community Edition"; exit 1; }
[ -z "$WANT" ] || grep -q "v$WANT-ce$" <<<"$ver" || { echo "expected v$WANT-ce"; exit 1; }

docker network create "$NET" >/dev/null
# The QuenchWorks postgresql image creates POSTGRES_DB only when it differs from
# the superuser name, so keep the default superuser and name the database.
docker run -d --name "$TAG-pg" --network "$NET" --network-alias pg \
  -e POSTGRES_PASSWORD="$PW" -e POSTGRES_DB=hasura "$PG" >/dev/null
for i in $(seq 1 60); do
  docker exec "$TAG-pg" pg_isready -h 127.0.0.1 -U postgres -d hasura >/dev/null 2>&1 && break
  [ "$i" = 60 ] && { echo "postgres never ready"; docker logs "$TAG-pg" | tail -20; exit 1; }
  sleep 1
done
docker run -d --name "$TAG-hge" --network "$NET" -p 127.0.0.1:18080:8080 \
  -e HASURA_GRAPHQL_DATABASE_URL="postgres://postgres:$PW@pg:5432/hasura" \
  -e HASURA_GRAPHQL_ADMIN_SECRET="$SECRET" "$IMAGE" >/dev/null
for i in $(seq 1 90); do
  [ "$(curl -fsS "$URL/healthz" 2>/dev/null)" = OK ] && break
  [ "$i" = 90 ] && { echo "/healthz never OK"; docker logs "$TAG-hge" 2>&1 | tail -40; exit 1; }
  sleep 2
done
echo "  /healthz: OK"

h=(-H "X-Hasura-Admin-Secret: $SECRET" -H 'Content-Type: application/json')
curl -fsS "${h[@]}" "$URL/v2/query" \
  -d '{"type":"run_sql","args":{"source":"default","sql":"CREATE TABLE smoke (id serial PRIMARY KEY, name text NOT NULL)"}}' >/dev/null
curl -fsS "${h[@]}" "$URL/v1/metadata" \
  -d '{"type":"pg_track_table","args":{"source":"default","table":"smoke"}}' >/dev/null
curl -fsS "${h[@]}" "$URL/v1/graphql" \
  -d '{"query":"mutation { insert_smoke_one(object: {name: \"quench\"}) { id } }"}' >/dev/null
got="$(curl -fsS "${h[@]}" "$URL/v1/graphql" -d '{"query":"{ smoke { name } }"}')"
echo "  query: $got"
grep -q '"name":"quench"' <<<"$got" || { echo "row not returned"; exit 1; }
anon="$(curl -sS -H 'Content-Type: application/json' "$URL/v1/graphql" -d '{"query":"{ smoke { name } }"}')"
grep -q '"errors"' <<<"$anon" || { echo "anonymous request was not refused: $anon"; exit 1; }
echo "  anonymous request refused"

echo "smoke test passed (hasura ${WANT:-?} CE: run_sql, track, GraphQL insert and query, admin secret; nonroot user: $user)"
