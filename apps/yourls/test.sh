#!/usr/bin/env bash
# Smoke test for a built YOURLS image. Usage: test.sh <image-ref> [version]
# Starts the catalog MariaDB and YOURLS on one network, runs the installer, shortens a URL
# through the API as the configured admin, and follows the short link to its redirect.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
DB_IMAGE="${YOURLS_TEST_DB_IMAGE:-ghcr.io/quenchworks/images/mariadb:11.8.9}"
NET="quench-yourls-net-$$" DB="quench-yourls-db-$$" APP="quench-yourls-smoke-$$"
PW="dbpw-$$" ADMIN_PW="adminpw-$$"
cleanup() { docker rm -f "$APP" "$DB" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
trap cleanup EXIT

docker network create "$NET" >/dev/null
docker run -d --name "$DB" --network "$NET" -e MARIADB_ROOT_PASSWORD="$PW" \
  -e MARIADB_DATABASE=yourls -e MARIADB_USER=yourls -e MARIADB_PASSWORD="$PW" "$DB_IMAGE" >/dev/null
for i in $(seq 1 60); do
  docker exec "$DB" mariadb-admin --socket=/run/mysqld/mysqld.sock -uroot -p"$PW" ping >/dev/null 2>&1 && break
  [ "$i" = 60 ] && { echo "mariadb did not start"; docker logs "$DB" | tail -20; exit 1; }
  sleep 2
done

docker run -d --name "$APP" --network "$NET" -p 127.0.0.1:8080:8080 \
  -e YOURLS_DB_HOST="$DB" -e YOURLS_DB_USER=yourls -e YOURLS_DB_PASS="$PW" \
  -e YOURLS_SITE=http://127.0.0.1:8080 -e YOURLS_COOKIEKEY="cookie-$$" \
  -e YOURLS_USER=admin -e YOURLS_PASS="$ADMIN_PW" "$IMAGE" >/dev/null
for i in $(seq 1 30); do
  [ "$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/admin/install.php)" = 200 ] && break
  [ "$i" = 30 ] && { echo "YOURLS did not answer"; docker logs "$APP" | tail -20; exit 1; }
  sleep 1
done

# create the tables (install.php needs no login before the first install)
page="$(curl -fsS -d install=1 http://127.0.0.1:8080/admin/install.php)"
grep -qi "tables.*created\|installed" <<<"$page" || { echo "install failed"; sed -n 1,40p <<<"$page"; exit 1; }

out="$(curl -fsS 'http://127.0.0.1:8080/yourls-api.php' --data-urlencode 'url=https://quench-works.com/smoke' \
  -d action=shorturl -d keyword=qwsmoke -d format=json -d username=admin -d password="$ADMIN_PW")"
grep -q '"statusCode":200' <<<"$out" || grep -q '"status":"success"' <<<"$out" || { echo "shorten failed: $out"; exit 1; }
loc="$(curl -s -o /dev/null -w '%{http_code} %{redirect_url}' http://127.0.0.1:8080/qwsmoke)"
[ "$loc" = "301 https://quench-works.com/smoke" ] || { echo "redirect wrong: '$loc'"; exit 1; }
echo "short link redirects: $loc"

# the API refuses a wrong password
bad="$(curl -s 'http://127.0.0.1:8080/yourls-api.php' -d action=stats -d format=json -d username=admin -d password=wrong)"
grep -q '"errorCode":"\?403' <<<"$bad" || { echo "bad password not refused: $bad"; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed (user $user)"
