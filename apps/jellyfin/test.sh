#!/usr/bin/env bash
# Smoke test for a built Jellyfin image. Usage: test.sh <image-ref> [version]
# On a read-only root with /config and /cache as volumes: the server must report healthy,
# serve its public info with the expected version, ship a working ffmpeg, and complete
# the first-run wizard through the API, after which a login must return an access token
# and a wrong password must be refused.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="quench-jellyfin-smoke-$$"
PORT=18096
B="http://127.0.0.1:$PORT"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

ff="$(docker run --rm --entrypoint /usr/bin/ffmpeg "$IMAGE" -version 2>&1 | head -1)"
grep -q '^ffmpeg version' <<<"$ff" || { echo "ffmpeg missing: $ff"; exit 1; }
echo "  $ff" | cut -c1-80

docker run -d --name "$NAME" --read-only --tmpfs /tmp \
  --tmpfs /config:uid=1001,gid=1001 --tmpfs /cache:uid=1001,gid=1001 \
  -p "127.0.0.1:$PORT:8096" "$IMAGE" >/dev/null
for i in $(seq 1 90); do
  out="$(curl -s -m 3 "$B/health" || true)"
  [ "$out" = "Healthy" ] && break
  [ "$i" = 90 ] && { echo "never healthy (last '$out')"; docker logs "$NAME" 2>&1 | tail -30; exit 1; }
  sleep 1
done
echo "  /health: $out"

info="$(curl -fsS "$B/System/Info/Public")"
ver="$(python3 -c 'import json,sys; print(json.load(sys.stdin)["Version"])' <<<"$info")"
echo "jellyfin $ver"
[ -z "$WANT" ] || [[ "$ver" == "$WANT"* ]] || { echo "expected $WANT"; exit 1; }
grep -q '"StartupWizardCompleted":false' <<<"$info" || { echo "not a fresh install: $info"; exit 1; }

# first-run wizard through the API
H='Content-Type: application/json'
auth='MediaBrowser Client="smoke", Device="test", DeviceId="smoke-1", Version="1"'
curl -fsS -H "$H" -H "Authorization: $auth" -d '{"UICulture":"en-US","MetadataCountryCode":"US","PreferredMetadataLanguage":"en"}' "$B/Startup/Configuration" >/dev/null
curl -fsS -H "Authorization: $auth" "$B/Startup/User" >/dev/null
curl -fsS -H "$H" -H "Authorization: $auth" -d '{"Name":"admin","Password":"smoke-pw-12345"}' "$B/Startup/User" >/dev/null
curl -fsS -X POST -H "Authorization: $auth" "$B/Startup/Complete" >/dev/null
tok="$(curl -fsS -H "$H" -H "Authorization: $auth" -d '{"Username":"admin","Pw":"smoke-pw-12345"}' "$B/Users/AuthenticateByName" | python3 -c 'import json,sys; print(json.load(sys.stdin)["AccessToken"])')"
[ -n "$tok" ] || { echo "no access token"; exit 1; }
echo "  wizard completed, admin logged in"
bad="$(curl -s -o /dev/null -w '%{http_code}' -H "$H" -H "Authorization: $auth" -d '{"Username":"admin","Pw":"wrong"}' "$B/Users/AuthenticateByName")"
[ "$bad" = 401 ] || { echo "a wrong password returned $bad, expected 401"; exit 1; }
echo "  a wrong password is refused"
if docker logs "$NAME" 2>&1 | grep -qiE 'read-only file system|unauthorizedaccess'; then
  echo "the server hit the read-only root:"; docker logs "$NAME" 2>&1 | grep -iE 'read-only|unauthorizedaccess' | head -5; exit 1
fi
echo "smoke test passed (jellyfin $ver, read-only rootfs, nonroot user: $user, ffmpeg present)"
