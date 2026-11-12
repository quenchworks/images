#!/usr/bin/env bash
# Smoke test for a built FreshRSS image. Usage: test.sh <image-ref> [version]
# The CLI sets up DATA_PATH on a volume (prepare, do-install, create-user with an API password),
# then FreshRSS runs on a read-only root. Through the Google Reader API: log in, refuse a wrong
# password, list the category an OPML import created, then restart and list it again. The web UI must render.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="freshrss-smoke-$$"
URL=http://127.0.0.1:18083
G="$URL/api/greader.php"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; docker volume rm "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
docker volume create "$NAME" >/dev/null
cli() { docker run --rm --read-only --tmpfs /tmp:uid=1001,gid=1001,mode=1777 -v "$NAME:/var/www/FreshRSS/data" --entrypoint php "$IMAGE" "$@"; }
cli ./cli/prepare.php >/dev/null
cli ./cli/do-install.php --default-user admin --auth-type form --environment production --base-url "$URL" \
  --db-type sqlite --api-enabled --disable-update >/dev/null
cli ./cli/create-user.php --user admin --password smoke-pass-2026 --api-password smoke-api-2026 >/dev/null
# a category with one subscription, imported as OPML (the feed is not fetched here)
docker run --rm -i -v "$NAME:/d" --entrypoint /bin/sh "$IMAGE" -c 'cat > /d/smoke.opml' <<'OPML'
<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0"><head><title>smoke</title></head><body>
<outline text="quench-smoke"><outline type="rss" text="Smoke feed" xmlUrl="http://feed.invalid/smoke.xml"/></outline>
</body></opml>
OPML
cli ./cli/import-for-user.php --user admin --filename /var/www/FreshRSS/data/smoke.opml >/dev/null
ver="$(cli -r 'require "constants.php"; echo FRESHRSS_VERSION;')"
echo "freshrss $ver"
[ -z "$WANT" ] || [ "$ver" = "$WANT" ] || { echo "expected $WANT"; exit 1; }

start() {
  docker run -d --name "$NAME" --read-only --tmpfs /tmp:uid=1001,gid=1001,mode=1777 -v "$NAME:/var/www/FreshRSS/data" \
    -p 127.0.0.1:18083:8080 "$IMAGE" >/dev/null
  for i in $(seq 1 60); do curl -fsS "$URL/i/" >/dev/null 2>&1 && return 0
    [ "$i" = 60 ] && { echo "web UI never answered"; docker logs "$NAME" 2>&1 | tail -30; exit 1; }; sleep 1; done
}
login() { curl -fsS "$G/accounts/ClientLogin" -d Email=admin -d "Passwd=$1" | sed -n "s/^Auth=//p"; }
start
curl -fsS "$URL/i/" | grep -qi 'freshrss' || { echo "web UI not rendered"; exit 1; }
code="$(curl -s -o /dev/null -w "%{http_code}" "$G/accounts/ClientLogin" -d Email=admin -d Passwd=wrong)"
[ "$code" = 401 ] || [ "$code" = 403 ] || { echo "a wrong API password got HTTP $code"; exit 1; }
tok="$(login smoke-api-2026)"; [ -n "$tok" ] || { echo "no API token"; exit 1; }
tags() { curl -fsS -H "Authorization: GoogleLogin auth=$(login smoke-api-2026)" "$G/reader/api/0/tag/list?output=json"; }
grep -q quench-smoke <<<"$(tags)" || { echo "category not listed: $(tags)"; exit 1; }
echo "  API login, wrong password refused ($code), imported category listed"
docker rm -f "$NAME" >/dev/null
start
grep -q quench-smoke <<<"$(tags)" || { echo "category lost across a restart"; exit 1; }
echo "  category kept across a restart"
if docker logs "$NAME" 2>&1 | grep -qiE 'read-only file system|PHP Fatal'; then echo "error in the logs"; docker logs "$NAME" 2>&1 | tail -20; exit 1; fi
echo "smoke test passed (freshrss ${WANT:-?}: CLI install on the data volume, Google Reader API login, imported category kept across a restart, read-only root; user $user)"
