#!/usr/bin/env bash
# Smoke test for a built Dependency-Track API server image. Usage: test.sh <image-ref> [version]
# Starts it against the QuenchWorks PostgreSQL image, then exercises the real API: the
# built-in admin must change its forced password, log in, create a project and read
# it back.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
TAG="quench-dtrack-smoke-$$"
NET="$TAG-net"
PG=ghcr.io/quenchworks/images/postgresql@sha256:4c0580fd2968b8d6e8fb008e0eb74b55c54464b041c235842e4d1cc37ed9a445
API=http://127.0.0.1:18080/api
PW="pg-$$-secret"

cleanup() { docker rm -f "$TAG-pg" "$TAG-api" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
trap cleanup EXIT
docker network create "$NET" >/dev/null

docker run -d --name "$TAG-pg" --network "$NET" --network-alias pg \
  -e POSTGRES_PASSWORD="$PW" -e POSTGRES_USER=dtrack -e POSTGRES_DB=dtrack "$PG" >/dev/null
# the server does not retry its database at startup, it exits: wait for PostgreSQL
for i in $(seq 1 60); do
  docker exec "$TAG-pg" pg_isready -h 127.0.0.1 -U dtrack -d dtrack >/dev/null 2>&1 && break
  [ "$i" = 60 ] && { echo "postgres never ready"; docker logs "$TAG-pg" | tail -20; exit 1; }
  sleep 1
done
docker run -d --name "$TAG-api" --network "$NET" -p 127.0.0.1:18080:8080 -p 127.0.0.1:19000:9000 \
  -e DT_DATASOURCE_URL="jdbc:postgresql://pg:5432/dtrack" \
  -e DT_DATASOURCE_USERNAME=dtrack -e DT_DATASOURCE_PASSWORD="$PW" "$IMAGE" >/dev/null

# schema migrations run on first start; allow a few minutes
for i in $(seq 1 90); do
  grep -q '"status" *: *"UP"' <<<"$(curl -fsS http://127.0.0.1:19000/health 2>/dev/null)" && break
  [ "$i" = 90 ] && { echo "never healthy"; docker logs "$TAG-api" | tail -40; exit 1; }
  sleep 2
done
echo "  /health: UP"

ver="$(curl -fsS "$API/version" | sed -n 's/.*"version" *: *"\([^"]*\)".*/\1/p')"
echo "reported version: $ver"
[ -n "$ver" ] || { echo "no version"; exit 1; }
[ -z "$WANT" ] || [ "$ver" = "$WANT" ] || { echo "expected $WANT"; exit 1; }

NEWPW="Quench-$$-Works!"
curl -fsS -o /dev/null -X POST "$API/v1/user/forceChangePassword" \
  --data-urlencode username=admin --data-urlencode password=admin \
  --data-urlencode newPassword="$NEWPW" --data-urlencode confirmPassword="$NEWPW" \
  || { echo "forced password change failed"; docker logs "$TAG-api" | tail -20; exit 1; }
token="$(curl -fsS -X POST "$API/v1/user/login" --data-urlencode username=admin --data-urlencode password="$NEWPW")"
[ -n "$token" ] || { echo "login returned no token"; exit 1; }
echo "  admin changed the forced password and logged in"

curl -fsS -o /dev/null -X PUT "$API/v1/project" -H "Authorization: Bearer $token" \
  -H 'Content-Type: application/json' -d '{"name":"quench-smoke","version":"1.0.0"}' \
  || { echo "project create failed"; docker logs "$TAG-api" | tail -20; exit 1; }
grep -q '"name" *: *"quench-smoke"' <<<"$(curl -fsS "$API/v1/project?name=quench-smoke" -H "Authorization: Bearer $token")" \
  || { echo "project not listed"; exit 1; }
echo "  created and read back project quench-smoke"

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (version $ver, nonroot user: $user, PostgreSQL, admin login, project create)"
