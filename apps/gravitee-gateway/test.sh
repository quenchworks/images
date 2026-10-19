#!/usr/bin/env bash
# Smoke test for a built Gravitee gateway image. Usage: test.sh <image-ref> [expected-version]
#
# Boots the gateway against a throwaway MongoDB (its management and rate-limit
# repository) on a read-only root with only /tmp writable, with OpenTelemetry tracing
# on so the swapped 1.62 SDK initializes. Proves: the gateway HTTP server answers with
# its own "no context-path" 404, the node API reports healthy, the plugins load
# (mongodb repository included) and no class from a swapped jar fails to link.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
WANT="${2:-}"
SFX="$$"; NET="quench-gw-net-$SFX"; DB="quench-gw-mongo-$SFX"; NAME="quench-gw-smoke-$SFX"
GW=18182; NODE=18183
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

docker run -d --name "$NAME" --network "$NET" --read-only --tmpfs /tmp:rw,mode=1777 \
  -e gravitee_repositories_management_mongodb_uri="mongodb://$DB:27017/gravitee" \
  -e gravitee_repositories_ratelimit_mongodb_uri="mongodb://$DB:27017/gravitee" \
  -e gravitee_management_mongodb_uri="mongodb://$DB:27017/gravitee" \
  -e gravitee_ratelimit_mongodb_uri="mongodb://$DB:27017/gravitee" \
  -e gravitee_reporters_elasticsearch_enabled=false \
  -e gravitee_services_core_http_host=0.0.0.0 \
  -e gravitee_services_opentelemetry_enabled=true \
  -e gravitee_services_opentelemetry_exporter_endpoint=http://127.0.0.1:4317 \
  -p "127.0.0.1:$GW:8082" -p "127.0.0.1:$NODE:18082" "$IMAGE" >/dev/null

body=""
for i in $(seq 1 120); do
  body="$(curl -sS --max-time 2 "http://127.0.0.1:$GW/" 2>/dev/null || true)"
  [ -n "$body" ] && break
  [ "$(docker inspect -f '{{.State.Status}}' "$NAME")" = running ] || fail "gateway exited"
  sleep 1
done
grep -q 'No context-path matches the request URI' <<<"$body" || fail "gateway answered '$body'"

# (4.11 reads the older management.* / ratelimit.* keys, 4.12 the repositories.* ones)
# the node API starts after the gateway HTTP server; every probe must report healthy
health=""
for i in $(seq 1 120); do
  health="$(curl -fsS --max-time 2 -u admin:adminadmin "http://127.0.0.1:$NODE/_node/health" 2>/dev/null || true)"
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
grep -q 'Initializing service io.gravitee.node.opentelemetry.tracer.OpenTelemetryTracer' <<<"$logs" || fail "OpenTelemetry tracer did not start"
if grep -E 'NoClassDefFoundError|ClassNotFoundException|NoSuchMethodError|NoSuchFieldError|AbstractMethodError|IncompatibleClassChangeError|UnsupportedClassVersionError' <<<"$logs"; then
  fail "a class failed to link"
fi
echo "smoke test passed (uid $user, gateway 404 page, node healthy, $n plugins)"
