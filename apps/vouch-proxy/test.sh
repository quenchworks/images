#!/usr/bin/env bash
# Smoke test for a built Vouch Proxy image. Usage: test.sh <image-ref> [version]
# Configured for a (dummy) OIDC provider through environment variables: /healthcheck answers,
# /validate refuses a request with no session cookie (401, what nginx auth_request acts on), and
# /login redirects to the provider's authorize URL with the configured client id.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="vouch-smoke-$$"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
v="$(docker run --rm "$IMAGE" -version)"
echo "vouch-proxy $v"
[ -z "$WANT" ] || [ "$v" = "v$WANT" ] || { echo "expected v$WANT"; exit 1; }

docker run -d --name "$NAME" --read-only -p 127.0.0.1:19090:9090 \
  -e VOUCH_DOMAINS=example.com -e VOUCH_JWT_SECRET=smoke-jwt-secret-0123456789abcdef \
  -e OAUTH_PROVIDER=oidc -e OAUTH_CLIENT_ID=smoke-client -e OAUTH_CLIENT_SECRET=smoke-secret \
  -e OAUTH_AUTH_URL=https://idp.example.com/authorize -e OAUTH_TOKEN_URL=https://idp.example.com/token \
  -e OAUTH_USER_INFO_URL=https://idp.example.com/userinfo -e OAUTH_SCOPES=openid,email \
  -e OAUTH_CALLBACK_URL=https://vouch.example.com/auth "$IMAGE" >/dev/null

ok=0
for _ in $(seq 1 30); do
  curl -fsS http://127.0.0.1:19090/healthcheck >/dev/null 2>&1 && { ok=1; break; }
  sleep 1
done
[ "$ok" = 1 ] || { echo "/healthcheck never answered"; docker logs "$NAME" 2>&1 | tail -20; exit 1; }
code="$(curl -s -o /dev/null -w '%{http_code}' -H 'Host: app.example.com' http://127.0.0.1:19090/validate)"
[ "$code" = 401 ] || { echo "/validate without a cookie got HTTP $code"; exit 1; }
loc="$(curl -s -o /dev/null -w '%{redirect_url}' -H 'Host: vouch.example.com' 'http://127.0.0.1:19090/login?url=https://app.example.com/')"
grep -q '^https://idp.example.com/authorize?.*client_id=smoke-client' <<<"$loc" || { echo "/login redirect: $loc"; docker logs "$NAME" 2>&1 | tail -10; exit 1; }
echo "smoke test passed: healthcheck, /validate 401 without a session, /login redirects to the IdP, user 1001"
