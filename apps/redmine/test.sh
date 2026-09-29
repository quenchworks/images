#!/usr/bin/env bash
# Smoke test for a built Redmine image. Usage: test.sh <image-ref> [version]
# Against the QuenchWorks PostgreSQL image, on a read-only root: migrates the schema,
# loads the default data, sets a known admin password through the app, then requires
# the login page to render, the REST API to accept that password and refuse a wrong one.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
TAG="quench-redmine-smoke-$$"
NET="$TAG-net"
PG=ghcr.io/quenchworks/images/postgresql@sha256:4c0580fd2968b8d6e8fb008e0eb74b55c54464b041c235842e4d1cc37ed9a445
DBPW="pg-$$-secret"; ADMINPW="Smoke-pw-$$-x"; SKB="$(head -c 48 /dev/urandom | od -An -tx1 | tr -d ' \n')"

cleanup() { docker rm -f "$TAG-pg" "$TAG-app" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
trap cleanup EXIT

# The QuenchWorks PostgreSQL image creates POSTGRES_DB only when it differs from POSTGRES_USER.
docker network create "$NET" >/dev/null
docker run -d --name "$TAG-pg" --network "$NET" --network-alias pg \
  -e POSTGRES_USER=redmine -e POSTGRES_PASSWORD="$DBPW" -e POSTGRES_DB=redminedb "$PG" >/dev/null
# pg_isready also passes on the temporary server the image starts to create the database,
# which then restarts; three real queries in a row mean the final server is up.
ok=0
for i in $(seq 1 90); do
  if docker exec -e PGPASSWORD="$DBPW" "$TAG-pg" psql -h 127.0.0.1 -U redmine -d redminedb -tAc 'select 1' >/dev/null 2>&1; then ok=$((ok+1)); else ok=0; fi
  [ "$ok" = 3 ] && break
  [ "$i" = 90 ] && { echo "postgres never ready"; docker logs "$TAG-pg" | tail -20; exit 1; }
  sleep 1
done

run() {  # one-shot command in the image, read-only root, writable /tmp and /data
  docker run --rm --network "$NET" --read-only --tmpfs /tmp --tmpfs /data:uid=1001,gid=1001 \
    -e REDMINE_DB_HOST=pg -e REDMINE_DB_NAME=redminedb -e REDMINE_DB_USER=redmine -e REDMINE_DB_PASSWORD="$DBPW" \
    -e SECRET_KEY_BASE="$SKB" "$IMAGE" "$@"
}
run rake db:migrate >/dev/null 2>&1 || { echo "db:migrate failed:"; run rake db:migrate 2>&1 | tail -20; exit 1; }
echo "  schema migrated"
REDMINE_LANG=en run rake redmine:load_default_data >/dev/null 2>&1 || true
ver="$(run runner 'Setting.rest_api_enabled = "1"; u = User.find_by_login("admin"); u.password = u.password_confirmation = ARGV[0]; u.must_change_passwd = false; u.save!; puts Redmine::VERSION.to_s' "$ADMINPW" 2>&1 | tail -1)"
ver="${ver%.stable}"   # Redmine::VERSION.to_s is "7.0.1.stable"
echo "redmine $ver"
[ -z "$WANT" ] || [ "$ver" = "$WANT" ] || { echo "expected $WANT"; exit 1; }

docker run -d --name "$TAG-app" --network "$NET" --read-only --tmpfs /tmp --tmpfs /data:uid=1001,gid=1001 \
  -p 127.0.0.1:13000:3000 \
  -e REDMINE_DB_HOST=pg -e REDMINE_DB_NAME=redminedb -e REDMINE_DB_USER=redmine -e REDMINE_DB_PASSWORD="$DBPW" \
  -e SECRET_KEY_BASE="$SKB" "$IMAGE" >/dev/null
for i in $(seq 1 90); do
  code="$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:13000/login || true)"
  [ "$code" = 200 ] && break
  [ "$i" = 90 ] && { echo "never served /login (last $code)"; docker logs "$TAG-app" 2>&1 | tail -30; exit 1; }
  sleep 1
done
page="$(curl -fsS http://127.0.0.1:13000/login)"
grep -q 'Redmine' <<<"$page" || { echo "login page did not render"; exit 1; }
echo "  /login renders"
me="$(curl -fsS -u "admin:$ADMINPW" http://127.0.0.1:13000/users/current.json)"
grep -q '"login":"admin"' <<<"$me" || { echo "API refused the admin: $me"; exit 1; }
bad="$(curl -s -o /dev/null -w '%{http_code}' -u "admin:wrong-password" http://127.0.0.1:13000/users/current.json)"
[ "$bad" = 401 ] || { echo "a wrong password returned $bad, expected 401"; exit 1; }
echo "  API accepts the admin password and refuses a wrong one"
if docker logs "$TAG-app" 2>&1 | grep -qiE 'read-only file system|permission denied'; then
  echo "the app hit the read-only root:"; docker logs "$TAG-app" 2>&1 | grep -iE 'read-only|permission denied' | head -5; exit 1
fi
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed (redmine $ver, read-only rootfs, nonroot user: $user)"
