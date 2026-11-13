#!/usr/bin/env bash
# Smoke test for a built Homebox image. Usage: test.sh <image-ref> [version]
# Homebox runs on a read-only root with /data on a volume. Through the REST API: register a user,
# refuse a wrong password, log in, create an entity (item) and read it back, then restart the container and
# read it again (SQLite on the volume). The web UI must be served from the embedded build.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="homebox-smoke-$$"
URL=http://127.0.0.1:17745
J=(-H 'Content-Type: application/json')
# required since 0.27 (>= 32 bytes): signs API keys
PEPPER="smoke-$$-pepper-0123456789abcdefghijklmnopqrstuv"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; docker volume rm "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
docker volume create "$NAME" >/dev/null
start() {
  docker run -d --name "$NAME" -e HBOX_AUTH_API_KEY_PEPPER="$PEPPER" --read-only --tmpfs /tmp:uid=1001,gid=1001 -v "$NAME:/data" -p 127.0.0.1:17745:7745 "$IMAGE" >/dev/null
  for i in $(seq 1 60); do curl -fsS "$URL/api/v1/status" >/dev/null 2>&1 && return 0
    [ "$i" = 60 ] && { echo "/api/v1/status never answered"; docker logs "$NAME" 2>&1 | tail -30; exit 1; }; sleep 1; done
}
start
st="$(curl -fsS "$URL/api/v1/status")"
ver="$(jq -er '.build.version' <<<"$st")"
echo "homebox $ver"
[ -z "$WANT" ] || [ "$ver" = "$WANT" ] || { echo "expected $WANT"; exit 1; }
curl -fsS "$URL/" | grep -qi '<html' || { echo "web UI not served"; exit 1; }

curl -fsS "${J[@]}" "$URL/api/v1/users/register" -d '{"name":"Smoke","email":"smoke@quench.test","password":"smoke-pass-2026"}' >/dev/null
code="$(curl -s -o /dev/null -w '%{http_code}' "${J[@]}" "$URL/api/v1/users/login" -d '{"username":"smoke@quench.test","password":"wrong-pass"}')"
[ "$code" = 401 ] || [ "$code" = 403 ] || { echo "a wrong password got HTTP $code"; exit 1; }
login() { curl -fsS "${J[@]}" "$URL/api/v1/users/login" -d '{"username":"smoke@quench.test","password":"smoke-pass-2026"}' \
  | jq -er '.token'; }
tok="$(login)"
A=(-H "Authorization: $tok" "${J[@]}")
echo "  registered, wrong password refused ($code), logged in"
curl -fsS "${A[@]}" "$URL/api/v1/entities" -d '{"name":"quench-smoke-item","description":"from the smoke test"}' >/dev/null
items() { curl -fsS "${A[@]}" "$URL/api/v1/entities?q=quench-smoke-item"; }
grep -q quench-smoke-item <<<"$(items)" || { echo "item not listed"; exit 1; }
echo "  item created and listed"

docker rm -f "$NAME" >/dev/null
start
tok="$(login)"; A=(-H "Authorization: $tok" "${J[@]}")
grep -q quench-smoke-item <<<"$(items)" || { echo "item lost across a restart"; exit 1; }
echo "  item kept across a restart"
if docker logs "$NAME" 2>&1 | grep -qiE 'read-only file system|panic'; then echo "crashed or wrote to the root"; docker logs "$NAME" 2>&1 | tail -20; exit 1; fi
echo "smoke test passed (homebox $ver: register, login, item kept across a restart on the /data volume, read-only root; user $user)"
