#!/usr/bin/env bash
# Smoke test for a built Actual image. Usage: test.sh <image-ref> [version]
# Actual runs on a read-only root with /data on a volume. Through the sync server API: set the
# server password (bootstrap), refuse a wrong one, log in, upload a budget file and list it, then
# restart and list it again. The web client must be served.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="actual-smoke-$$"
URL=http://127.0.0.1:15006
J=(-H 'Content-Type: application/json')
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; docker volume rm "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
ver="$(docker run --rm --entrypoint /usr/bin/node "$IMAGE" -p 'require("/app/node_modules/@actual-app/sync-server/package.json").version')"
echo "actual $ver"
[ -z "$WANT" ] || [ "$ver" = "$WANT" ] || { echo "expected $WANT"; exit 1; }
docker volume create "$NAME" >/dev/null
start() {
  docker run -d --name "$NAME" --read-only --tmpfs /tmp:uid=1001,gid=1001 -v "$NAME:/data" -p 127.0.0.1:15006:5006 "$IMAGE" >/dev/null
  for i in $(seq 1 60); do curl -fsS "$URL/health" >/dev/null 2>&1 && return 0
    [ "$i" = 60 ] && { echo "/health never answered"; docker logs "$NAME" 2>&1 | tail -30; exit 1; }; sleep 1; done
}
field() { jq -er --arg k "$1" '.data[$k]'; }
start
curl -fsS "$URL/" | grep -qi '<html' || { echo "web client not served"; exit 1; }
curl -fsS "${J[@]}" "$URL/account/bootstrap" -d '{"password":"smoke-pass-2026"}' >/dev/null
bad="$(curl -sS "${J[@]}" "$URL/account/login" -d '{"loginMethod":"password","password":"wrong"}')"
grep -q '"status":"error"' <<<"$bad" || { echo "a wrong password was accepted: $bad"; exit 1; }
login() { curl -fsS "${J[@]}" "$URL/account/login" -d '{"loginMethod":"password","password":"smoke-pass-2026"}' | field token; }
tok="$(login)"
echo "  bootstrapped, wrong password refused, logged in"
curl -fsS -H "x-actual-token: $tok" -H 'Content-Type: application/encrypted-file' -H 'x-actual-file-id: quench-smoke-file' \
  -H 'x-actual-name: quench-smoke' -H 'x-actual-format: 2' --data-binary 'quench-smoke-budget' "$URL/sync/upload-user-file" >/dev/null
files() { curl -fsS -H "x-actual-token: $(login)" "$URL/sync/list-user-files"; }
grep -q quench-smoke <<<"$(files)" || { echo "file not listed: $(files)"; exit 1; }
echo "  budget file uploaded and listed"
docker rm -f "$NAME" >/dev/null
start
grep -q quench-smoke <<<"$(files)" || { echo "file lost across a restart"; exit 1; }
echo "  file and server password kept across a restart"
if docker logs "$NAME" 2>&1 | grep -qiE 'read-only file system|EROFS'; then echo "wrote to the root"; docker logs "$NAME" 2>&1 | tail -20; exit 1; fi
echo "smoke test passed (actual ${WANT:-?}: bootstrap, login, budget file kept across a restart on /data, read-only root; user $user)"
