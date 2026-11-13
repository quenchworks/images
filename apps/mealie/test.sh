#!/usr/bin/env bash
# Smoke test for a built Mealie image. Usage: test.sh <image-ref> [version]
# Mealie runs on a read-only root with /app/data on a volume (SQLite). Through the API: the
# release's version, log in as the default admin, refuse a wrong password, create a recipe and
# read it back, then restart and read it again. The generated frontend must be served.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="mealie-smoke-$$"
URL=http://127.0.0.1:19000
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; docker volume rm "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
docker volume create "$NAME" >/dev/null
start() {
  docker run -d --name "$NAME" --read-only --tmpfs /tmp:uid=1001,gid=1001 -v "$NAME:/app/data" -p 127.0.0.1:19000:9000 "$IMAGE" >/dev/null
  for i in $(seq 1 120); do curl -fsS "$URL/api/app/about" >/dev/null 2>&1 && return 0
    [ "$i" = 120 ] && { echo "/api/app/about never answered"; docker logs "$NAME" 2>&1 | tail -30; exit 1; }; sleep 1; done
}
login() { curl -fsS "$URL/api/auth/token" -d username=changeme@example.com -d "password=$1" \
  | jq -er '.access_token'; }
start
ver="$(curl -fsS "$URL/api/app/about" | jq -er '.version')"
echo "mealie $ver"
[ -z "$WANT" ] || [ "${ver#v}" = "$WANT" ] || { echo "expected $WANT"; exit 1; }
curl -fsS "$URL/" | grep -qi '<html' || { echo "frontend not served"; exit 1; }
code="$(curl -s -o /dev/null -w '%{http_code}' "$URL/api/auth/token" -d username=changeme@example.com -d password=wrong)"
[ "$code" = 401 ] || { echo "a wrong password got HTTP $code"; exit 1; }
tok="$(login MyPassword)"
slug="$(curl -fsS -H "Authorization: Bearer $tok" -H 'Content-Type: application/json' "$URL/api/recipes" -d '{"name":"Quench Smoke Soup"}' | tr -d '"')"
[ -n "$slug" ] || { echo "no recipe slug"; exit 1; }
recipe() { curl -fsS -H "Authorization: Bearer $(login MyPassword)" "$URL/api/recipes/$slug"; }
grep -q 'Quench Smoke Soup' <<<"$(recipe)" || { echo "recipe not readable"; exit 1; }
echo "  admin login, wrong password refused, recipe $slug created"
docker rm -f "$NAME" >/dev/null
start
grep -q 'Quench Smoke Soup' <<<"$(recipe)" || { echo "recipe lost across a restart"; exit 1; }
echo "  recipe kept across a restart"
if docker logs "$NAME" 2>&1 | grep -qiE 'read-only file system|Traceback'; then echo "error in the logs"; docker logs "$NAME" 2>&1 | tail -20; exit 1; fi
echo "smoke test passed (mealie $ver: API login, recipe kept across a restart on /app/data, frontend served, read-only root; user $user)"
