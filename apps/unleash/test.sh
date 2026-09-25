#!/usr/bin/env bash
# Smoke test for a built unleash image. Usage: test.sh <image-ref> [version]
# Real feature-flag work: Unleash starts on a read-only root against the QuenchWorks
# postgresql image, migrates the schema, creates the admin from env, and that admin logs
# in, creates a flag in the default project and reads it back through the admin API.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NET="unleash-smoke-$$"; PG="$NET-pg"; NAME="$NET-unleash"
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
cleanup() { docker rm -f "$NAME" "$PG" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; rm -rf "$WORK"; }
trap cleanup EXIT
trap 'echo "failed at line $LINENO"; docker logs "$NAME" 2>&1 | tail -30' ERR

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

PW="pg-$$-smoke"
docker network create "$NET" >/dev/null
docker run -d --name "$PG" --network "$NET" -e POSTGRES_PASSWORD="$PW" ghcr.io/quenchworks/images/postgresql:18.6 >/dev/null
for i in $(seq 1 60); do
  docker exec -e PGPASSWORD="$PW" "$PG" psql -h 127.0.0.1 -U postgres -tAc 'select 1' >/dev/null 2>&1 && break
  [ "$i" = 60 ] && { echo "postgres did not become ready"; docker logs "$PG" 2>&1 | tail -20; exit 1; }
  sleep 2
done

ADMIN_PW="Quench-smoke-$$-Pw1"
docker run -d --name "$NAME" --network "$NET" --read-only --tmpfs /tmp -p 127.0.0.1:14242:4242 \
  -e DATABASE_URL="postgres://postgres:$PW@$PG:5432/postgres" -e DATABASE_SSL=false \
  -e UNLEASH_DEFAULT_ADMIN_USERNAME=admin -e UNLEASH_DEFAULT_ADMIN_PASSWORD="$ADMIN_PW" \
  -e CHECK_VERSION=false -e SEND_TELEMETRY=false "$IMAGE" >/dev/null
B=http://127.0.0.1:14242
ok=0
for _ in $(seq 1 120); do curl -fsS "$B/health" 2>/dev/null | grep -q '"health":"GOOD"' && { ok=1; break; }; sleep 1; done
[ "$ok" = 1 ] || { echo "unleash never reported healthy"; docker logs "$NAME" 2>&1 | tail -40; exit 1; }

J=(-H 'Content-Type: application/json')
curl -fsS -c "$WORK/c" "${J[@]}" -d "{\"username\":\"admin\",\"password\":\"$ADMIN_PW\"}" "$B/auth/simple/login" >/dev/null
# the admin UI config (a signed-in route) reports the running version
[ -z "$WANT" ] || curl -fsS -b "$WORK/c" "$B/api/admin/ui-config" | grep -q "\"version\":\"$WANT\"" \
  || { echo "version $WANT not reported"; curl -sS -b "$WORK/c" "$B/api/admin/ui-config" | head -c 300; exit 1; }
curl -fsS -b "$WORK/c" "${J[@]}" -d '{"name":"quench-smoke","type":"release"}' \
  "$B/api/admin/projects/default/features" >/dev/null
curl -fsS -b "$WORK/c" "$B/api/admin/projects/default/features/quench-smoke" | grep -q '"name":"quench-smoke"' \
  || { echo "flag not read back"; exit 1; }
# a wrong password is refused
[ "$(curl -s -o /dev/null -w '%{http_code}' "${J[@]}" -d '{"username":"admin","password":"wrong"}' "$B/auth/simple/login")" -ge 400 ]
if docker logs "$NAME" 2>&1 | grep -E '"level":"(error|fatal)"|\[ERROR\]|FATAL'; then echo "unleash logged errors"; exit 1; fi
echo "smoke test passed (unleash ${WANT:-?}, uid $user, migrated, admin login, flag created and read)"
