#!/usr/bin/env bash
# Smoke test for a built Label Studio image. Usage: test.sh <image-ref> [version]
# Boots the server on SQLite with an admin user and API token from the
# environment, and requires /health UP, /api/version to report this release, a
# project created and listed through the token-authenticated API, and a bad
# token refused.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="quench-label-studio-smoke-$$"
PORT=18080
TOKEN=quenchsmoketoken1234
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

# 1.23 disables legacy API tokens by default; the env token is a legacy token.
docker run -d --name "$NAME" -p "127.0.0.1:$PORT:8080" \
  -e LABEL_STUDIO_ENABLE_LEGACY_API_TOKEN=true \
  -e LABEL_STUDIO_USERNAME=admin@example.com \
  -e LABEL_STUDIO_PASSWORD=quench-smoke-pass \
  -e LABEL_STUDIO_USER_TOKEN="$TOKEN" \
  "$IMAGE" >/dev/null
for i in $(seq 1 120); do
  health="$(curl -fsS "http://127.0.0.1:$PORT/health" 2>/dev/null || true)"
  grep -q '"UP"' <<<"$health" && break
  [ "$i" = 120 ] && { echo "/health never came UP"; docker logs "$NAME" 2>&1 | tail -40; exit 1; }
  sleep 2
done
echo "health: $health"

ver="$(curl -fsS "http://127.0.0.1:$PORT/api/version")"
release="$(python3 -c 'import json,sys; print(json.load(sys.stdin)["release"])' <<<"$ver")"
echo "label-studio $release"
[ -z "$WANT" ] || [ "$release" = "$WANT" ] || { echo "expected $WANT"; exit 1; }

curl -fsS -X POST "http://127.0.0.1:$PORT/api/projects" \
  -H "Authorization: Token $TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"smoke","label_config":"<View><Text name=\"t\" value=\"$text\"/><Choices name=\"c\" toName=\"t\"><Choice value=\"a\"/></Choices></View>"}' >/dev/null
projects="$(curl -fsS "http://127.0.0.1:$PORT/api/projects" -H "Authorization: Token $TOKEN")"
grep -q '"title": *"smoke"' <<<"$projects" || { echo "project not listed: $projects"; exit 1; }
code="$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$PORT/api/projects" -H 'Authorization: Token wrong-token')"
[ "$code" = 401 ] || { echo "bad token got HTTP $code, want 401"; exit 1; }

echo "smoke test passed (label-studio ${WANT:-?}: health, version, project API, token auth; nonroot user: $user)"
