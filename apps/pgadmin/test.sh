#!/usr/bin/env bash
# Smoke test for a built pgadmin image. Usage: test.sh <image-ref> [version]
# Real admin work on a read-only root with /var/lib/pgadmin on a volume: pgAdmin creates
# its admin on first start, that admin signs in (form + CSRF), registers the QuenchWorks
# postgresql image as a server with connect_now, which makes psycopg reach PostgreSQL
# through Wolfi's libpq, and lists the server's databases.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NET="pgadmin-smoke-$$"; PG="$NET-pg"; NAME="$NET-pgadmin"
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
cleanup() { docker rm -f "$NAME" "$PG" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; rm -rf "$WORK"; }
trap cleanup EXIT
trap 'echo "failed at line $LINENO"; docker logs "$NAME" 2>&1 | tail -30' ERR

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

ver="$(docker run --rm --entrypoint /opt/pgadmin/venv/bin/python "$IMAGE" -c "import importlib.metadata as m; print(m.version('pgadmin4'))")"
[ -z "$WANT" ] || [ "$ver" = "$WANT" ] || { echo "version mismatch: $ver"; exit 1; }

PW="pg-$$-smoke"
docker network create "$NET" >/dev/null
docker run -d --name "$PG" --network "$NET" -e POSTGRES_PASSWORD="$PW" ghcr.io/quenchworks/images/postgresql:18.6 >/dev/null
for i in $(seq 1 60); do
  docker exec -e PGPASSWORD="$PW" "$PG" psql -h 127.0.0.1 -U postgres -tAc 'select 1' >/dev/null 2>&1 && break
  [ "$i" = 60 ] && { echo "postgres did not become ready"; exit 1; }
  sleep 2
done

APW="Quench-smoke-$$-Pw1"
docker run -d --name "$NAME" --network "$NET" --read-only --tmpfs /tmp \
  --tmpfs /var/lib/pgadmin:uid=1001,gid=1001,mode=0700 -p 127.0.0.1:15050:5050 \
  -e PGADMIN_SETUP_EMAIL=admin@example.com -e PGADMIN_SETUP_PASSWORD="$APW" "$IMAGE" >/dev/null
B=http://127.0.0.1:15050
ok=0
for _ in $(seq 1 120); do [ "$(curl -fsS "$B/misc/ping" 2>/dev/null)" = PING ] && { ok=1; break; }; sleep 1; done
[ "$ok" = 1 ] || { echo "pgadmin never answered /misc/ping"; docker logs "$NAME" 2>&1 | tail -40; exit 1; }

C="$WORK/cookies"
tok="$(curl -fsS -c "$C" -b "$C" "$B/login" | sed -n 's/.*"csrfToken": *"\([^"]*\)".*/\1/p')"
[ -n "$tok" ] || { echo "no CSRF token on the login page"; exit 1; }
where="$(curl -sS -o /dev/null -w '%{redirect_url}' -c "$C" -b "$C" --data-urlencode email=admin@example.com \
  --data-urlencode "password=$APW" --data-urlencode "csrf_token=$tok" "$B/authenticate/login")"
case "$where" in */browser/) ;; *) echo "login went to '$where'"; exit 1 ;; esac
H=(-b "$C" -c "$C" -H "X-pgA-CSRFToken: $tok" -H 'Content-Type: application/json')
out="$(curl -fsS "${H[@]}" -d "{\"name\":\"smoke\",\"host\":\"$PG\",\"port\":5432,\"db\":\"postgres\",\"username\":\"postgres\",\"password\":\"$PW\",\"sslmode\":\"prefer\",\"connect_now\":true}" \
  "$B/browser/server/obj/1/")"
echo "$out" | grep -q '"connected": *true' || { echo "server not connected: $(echo "$out" | head -c 400)"; exit 1; }
sid="$(echo "$out" | sed -n 's/^{"node":{"_id": *"\{0,1\}\([0-9][0-9]*\).*/\1/p')"
[ -n "$sid" ] || { echo "no server id in: $(echo "$out" | head -c 200)"; exit 1; }
dbs="$(curl -sS "${H[@]}" "$B/browser/database/nodes/1/$sid/")"
echo "$dbs" | grep -q '"label": *"postgres"' || { echo "databases not listed (server id '$sid'): $(echo "$dbs" | head -c 400)"; echo "create: $(echo "$out" | head -c 300)"; exit 1; }
if docker logs "$NAME" 2>&1 | grep -E 'Traceback|CRITICAL|ERROR'; then echo "pgadmin logged errors"; exit 1; fi
echo "smoke test passed (pgadmin ${WANT:-?}, uid $user, admin login, server connected through libpq, databases listed)"
