#!/usr/bin/env bash
# Smoke test for a built BookStack image. Usage: test.sh <image-ref> [version]
# BookStack runs on a read-only root against the QuenchWorks MariaDB image, with storage/ and
# public/uploads on volumes. supervisord migrates the schema at start. Through the web UI (CSRF
# token and session cookie, as a browser does): a wrong password is refused, the seeded admin logs
# in and creates a book, which is still there after a restart.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
TAG="bookstack-smoke-$$"
NET="$TAG-net"
DB=ghcr.io/quenchworks/images/mariadb@sha256:394b66879fa725ea4ab27d7b3beb6e556fcf0ee853241800d6eebf4ff7e7958d
PW="db-$$-secret"
URL=http://127.0.0.1:18084
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
cleanup() { docker rm -f "$TAG-db" "$TAG-app" >/dev/null 2>&1 || true; docker volume rm "$TAG-st" "$TAG-up" >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true; rm -rf "$WORK"; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
ver="$(docker run --rm --entrypoint /bin/cat "$IMAGE" /var/www/bookstack/version)"
echo "bookstack $ver"
[ -z "$WANT" ] || [ "$ver" = "v$WANT" ] || { echo "expected v$WANT"; exit 1; }

docker network create "$NET" >/dev/null
docker run -d --name "$TAG-db" --network "$NET" --network-alias db -e MARIADB_ROOT_PASSWORD="$PW" \
  -e MARIADB_DATABASE=bookstack -e MARIADB_USER=bookstack -e MARIADB_PASSWORD="$PW" "$DB" >/dev/null
for i in $(seq 1 60); do docker exec "$TAG-db" mariadb -ubookstack -p"$PW" -e 'select 1' bookstack >/dev/null 2>&1 && break
  [ "$i" = 60 ] && { echo "mariadb never ready"; docker logs "$TAG-db" 2>&1 | tail -20; exit 1; }; sleep 1; done
docker volume create "$TAG-st" >/dev/null; docker volume create "$TAG-up" >/dev/null
key="base64:$(head -c 32 /dev/urandom | base64)"
start() {
  docker run -d --name "$TAG-app" --network "$NET" --read-only --tmpfs /tmp:uid=1001,gid=1001,mode=1777 \
    -v "$TAG-st:/var/www/bookstack/storage" -v "$TAG-up:/var/www/bookstack/public/uploads" \
    -e APP_KEY="$key" -e APP_URL="$URL" -e DB_HOST=db -e DB_DATABASE=bookstack -e DB_USERNAME=bookstack -e DB_PASSWORD="$PW" \
    -p 127.0.0.1:18084:8080 "$IMAGE" >/dev/null
  for i in $(seq 1 90); do curl -fsS "$URL/login" 2>/dev/null | grep -q '_token' && return 0
    [ "$i" = 90 ] && { echo "/login never served"; docker logs "$TAG-app" 2>&1 | tail -30; exit 1; }; sleep 1; done
}
J="$WORK/jar"
login() { # $1 password; prints the HTTP status of the login POST, keeps the session in $J
  rm -f "$J"; tok="$(curl -fsS -c "$J" "$URL/login" | sed -n 's/.*name="_token" value="\([^"]*\)".*/\1/p' | head -1)"
  curl -s -o /dev/null -w '%{redirect_url}' -b "$J" -c "$J" "$URL/login" --data-urlencode "_token=$tok" \
    --data-urlencode email=admin@admin.com --data-urlencode "password=$1"; }
start
r="$(login wrong-password)"; case "$r" in */login) ;; *) echo "a wrong password redirected to '$r'"; exit 1;; esac
r="$(login password)"; case "$r" in */login|"") echo "admin login failed: '$r'"; docker logs "$TAG-app" 2>&1 | tail -20; exit 1;; esac
tok="$(curl -fsS -b "$J" -c "$J" "$URL/create-book" | sed -n 's/.*name="_token" value="\([^"]*\)".*/\1/p' | head -1)"
curl -fsS -o /dev/null -b "$J" -c "$J" "$URL/books" --data-urlencode "_token=$tok" --data-urlencode 'name=Quench Smoke Book' \
  --data-urlencode 'description_html=<p>smoke</p>'
book() { login password >/dev/null; curl -fsS -b "$J" "$URL/books/quench-smoke-book"; }
grep -q 'Quench Smoke Book' <<<"$(book)" || { echo "book not readable"; exit 1; }
echo "  migrated, wrong password refused, admin logged in, book created"
docker rm -f "$TAG-app" >/dev/null
start
grep -q 'Quench Smoke Book' <<<"$(book)" || { echo "book lost across a restart"; exit 1; }
echo "  book kept across a restart"
if docker logs "$TAG-app" 2>&1 | grep -qiE 'read-only file system|PHP Fatal'; then echo "error in the logs"; docker logs "$TAG-app" 2>&1 | tail -20; exit 1; fi
echo "smoke test passed (bookstack $ver: migrate on MariaDB, web login with CSRF, book kept across a restart, read-only root; user $user)"
