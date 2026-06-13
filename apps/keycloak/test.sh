#!/usr/bin/env bash
# Smoke test for a built Keycloak image. Usage: test.sh <image-ref>
# Runs the OPTIMIZED production start (`kc.sh start --optimized`) with a READ-ONLY
# rootfs + writable tmpfs at /data, /conf and /tmp to prove the entrypoint relocates
# the data dir / conf / tmp correctly. We bake the optimized image for --db=postgres
# (the vendor the chart wires), so the self-contained test spins up a throwaway Postgres
# sidecar. (start-dev cannot be used for the gate: it re-augments at boot and writes
# into the read-only /opt/keycloak/lib/quarkus tree.) Then we exercise the management
# health port, the main HTTP listener, the admin console, and an admin token.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
SFX="$$"
NET="quench-kc-net-$SFX"
PG="quench-kc-pg-$SFX"
NAME="quench-keycloak-smoke-$SFX"
HTTP="http://127.0.0.1:8080"
MGMT="http://127.0.0.1:9000"

cleanup() {
  docker rm -f "$NAME" "$PG" >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker network create "$NET" >/dev/null

echo "starting throwaway Postgres sidecar"
docker run -d --name "$PG" --network "$NET" \
  -e POSTGRES_DB=keycloak -e POSTGRES_USER=keycloak -e POSTGRES_PASSWORD=keycloak \
  postgres:16-alpine >/dev/null
# wait for Postgres to accept connections
for i in $(seq 1 30); do
  if docker exec "$PG" pg_isready -U keycloak >/dev/null 2>&1; then break; fi
  [ "$i" = 30 ] && { echo "postgres did not become ready"; docker logs "$PG" | tail -20; exit 1; }
  sleep 1
done

echo "starting $IMAGE (read-only rootfs + writable /data,/conf,/tmp tmpfs; optimized start, postgres)"
docker run -d --name "$NAME" --network "$NET" \
  --read-only \
  --tmpfs /data:rw,mode=1777 \
  --tmpfs /conf:rw,mode=1777 \
  --tmpfs /tmp:rw,mode=1777 \
  -e KC_DB=postgres \
  -e KC_DB_URL="jdbc:postgresql://${PG}:5432/keycloak" \
  -e KC_DB_USERNAME=keycloak -e KC_DB_PASSWORD=keycloak \
  -e KC_BOOTSTRAP_ADMIN_USERNAME=admin \
  -e KC_BOOTSTRAP_ADMIN_PASSWORD=admin \
  -p 127.0.0.1:8080:8080 \
  -p 127.0.0.1:9000:9000 \
  "$IMAGE" >/dev/null

# Quarkus + DB init takes a while; poll the management health/ready endpoint (port 9000).
for i in $(seq 1 90); do
  if curl -fsS "${MGMT}/health/ready" 2>/dev/null | grep -q '"status": *"UP"'; then break; fi
  if ! docker ps --format '{{.Names}}' | grep -q "^${NAME}\$"; then
    echo "keycloak container exited"; docker logs "$NAME" | tail -60; exit 1
  fi
  [ "$i" = 90 ] && { echo "keycloak did not become ready"; docker logs "$NAME" | tail -60; exit 1; }
  sleep 2
done
echo "health/ready is UP:"; curl -fsS "${MGMT}/health/ready" 2>/dev/null | grep -o '"status": *"UP"' | head -1

# the main HTTP listener (8080) should serve the welcome page / redirect
echo "checking main HTTP (8080)"
code="$(curl -fsS -o /dev/null -w '%{http_code}' "${HTTP}/" 2>/dev/null || true)"
case "$code" in
  200|302|303) echo "  GET / -> $code (ok)";;
  *) echo "  GET / unexpected status $code"; docker logs "$NAME" | tail -40; exit 1;;
esac

# the admin console should load
echo "checking admin console"
code="$(curl -fsS -o /dev/null -w '%{http_code}' "${HTTP}/admin/master/console/" 2>/dev/null || true)"
[ "$code" = "200" ] || { echo "  admin console returned $code"; exit 1; }
echo "  GET /admin/master/console/ -> 200 (ok)"

# prove the admin bootstrap creds work: get an admin token from the master realm
echo "obtaining an admin token (proves KC_BOOTSTRAP_ADMIN_* worked)"
tok="$(curl -fsS -X POST "${HTTP}/realms/master/protocol/openid-connect/token" \
  -d 'grant_type=password' -d 'client_id=admin-cli' \
  -d 'username=admin' -d 'password=admin' 2>/dev/null | grep -o '"access_token":"[^"]*"' || true)"
[ -n "$tok" ] || { echo "  admin token request failed"; docker logs "$NAME" | tail -40; exit 1; }
echo "  admin token acquired (ok)"

# must run as the nonroot keycloak user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
