#!/usr/bin/env bash
# Smoke test for a built Gravitee management API image. Usage: test.sh <image-ref> [expected-version]
#
# Boots the management API against a throwaway MongoDB on a read-only root with only
# /tmp writable and OpenTelemetry tracing on. Proves: the default admin logs in and gets
# a token, the token lists APIs on the v2 management API, the portal API answers, the
# node API reports healthy, the mongodb repository loads and no class from a swapped
# jar fails to link.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
WANT="${2:-}"
SFX="$$"; NET="quench-mapi-net-$SFX"; DB="quench-mapi-mongo-$SFX"; NAME="quench-mapi-smoke-$SFX"
API=18283; NODE=18284
B="http://127.0.0.1:$API"
cleanup() { docker rm -f "$NAME" "$DB" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
trap cleanup EXIT
fail() { echo "FAIL: $*"; docker logs "$NAME" 2>&1 | tail -60; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

docker network create "$NET" >/dev/null
docker run -d --name "$DB" --network "$NET" mongo:8.0 >/dev/null
for i in $(seq 1 60); do
  docker exec "$DB" mongosh --quiet --eval 'db.runCommand({ping:1}).ok' >/dev/null 2>&1 && break
  [ "$i" = 60 ] && { echo "mongodb did not start"; docker logs "$DB" | tail -20; exit 1; }
  sleep 1
done

# 4.11 reads the older management.* keys, 4.12 the repositories.* ones: set both
docker run -d --name "$NAME" --network "$NET" --read-only --tmpfs /tmp:rw,mode=1777 \
  -e gravitee_repositories_management_mongodb_uri="mongodb://$DB:27017/gravitee" \
  -e gravitee_management_mongodb_uri="mongodb://$DB:27017/gravitee" \
  -e gravitee_repositories_analytics_type=none -e gravitee_analytics_type=none \
  -e gravitee_services_core_http_host=0.0.0.0 \
  -e gravitee_services_opentelemetry_enabled=true \
  -e gravitee_services_opentelemetry_exporter_endpoint=http://127.0.0.1:4317 \
  -p "127.0.0.1:$API:8083" -p "127.0.0.1:$NODE:18083" "$IMAGE" >/dev/null

tok=""
for i in $(seq 1 180); do
  out="$(curl -sS --max-time 3 -u admin:admin -X POST "$B/management/organizations/DEFAULT/user/login" 2>/dev/null || true)"
  tok="$(sed -n 's/.*"token" *: *"\([^"]*\)".*/\1/p' <<<"$out")"
  [ -n "$tok" ] && break
  [ "$(docker inspect -f '{{.State.Status}}' "$NAME")" = running ] || fail "management API exited"
  sleep 1
done
[ -n "$tok" ] || fail "admin login never returned a token (last: $out)"
echo "admin login ok"

apis="$(curl -fsS -H "Authorization: Bearer $tok" "$B/management/v2/environments/DEFAULT/apis")" || fail "v2 api list failed"
grep -q '"data"' <<<"$apis" || fail "v2 api list returned '$apis'"
me="$(curl -fsS -H "Authorization: Bearer $tok" "$B/management/organizations/DEFAULT/user")" || fail "current user failed"
grep -q '"displayName" *: *"admin"' <<<"$me" || fail "current user is not admin: $me"
portal="$(curl -fsS "$B/portal/environments/DEFAULT/configuration")" || fail "portal API did not answer"
grep -q '"portal"' <<<"$portal" || fail "portal configuration returned '$portal'"

health=""
for i in $(seq 1 60); do
  health="$(curl -fsS --max-time 3 -u admin:adminadmin "http://127.0.0.1:$NODE/_node/health" 2>/dev/null || true)"
  grep -q '"management-repository" *: *{ *"healthy" *: *true' <<<"$health" && ! grep -q '"healthy" *: *false' <<<"$health" && break
  sleep 1
done
echo "health: $health"
grep -q '"management-repository" *: *{ *"healthy" *: *true' <<<"$health" || fail "management repository not healthy"
! grep -q '"healthy" *: *false' <<<"$health" || fail "a health probe is down"
node="$(curl -fsS -u admin:adminadmin "http://127.0.0.1:$NODE/_node")" || fail "/_node did not answer"
[ -z "$WANT" ] || grep -q "\"MAJOR_VERSION\" *: *\"$WANT\"" <<<"$node" || fail "expected version $WANT in $node"

logs="$(docker logs "$NAME" 2>&1)"
grep -q 'Repository \[MANAGEMENT\] loaded by mongodb' <<<"$logs" || fail "mongodb repository plugin not loaded"
n="$(grep -c 'has been loaded' <<<"$logs" || true)"
echo "plugins loaded: $n"
[ "$n" -gt 60 ] || fail "only $n plugins loaded"
if grep -E 'NoClassDefFoundError|ClassNotFoundException|NoSuchMethodError|NoSuchFieldError|AbstractMethodError|IncompatibleClassChangeError|UnsupportedClassVersionError|Read-only file system' <<<"$logs"; then
  fail "a class failed to link or a write hit the read-only root"
fi
echo "smoke test passed (uid $user, admin token, v2 API list, portal, node healthy, $n plugins)"
