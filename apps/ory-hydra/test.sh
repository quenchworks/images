#!/usr/bin/env bash
# Smoke test for a built ory-hydra image. Usage: test.sh <image-ref> [version]
# Real OAuth work, not a banner: serve all on an in-memory store, register a client
# through the admin API, and mint a client_credentials token on the public API, then
# introspect it on the admin API.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="hydra-smoke-$$"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

ver="$(docker run --rm "$IMAGE" version 2>&1)"
echo "$ver"
[ -z "$WANT" ] || echo "$ver" | grep -q "v\?${WANT}" || { echo "expected version $WANT"; exit 1; }

docker run -d --name "$NAME" --read-only --tmpfs /tmp \
  -e DSN=memory -e SECRETS_SYSTEM=quenchworks-smoke-secret-0123456789 \
  -e URLS_SELF_ISSUER=http://127.0.0.1:14444 \
  -p 127.0.0.1:14444:4444 -p 127.0.0.1:14445:4445 "$IMAGE" serve all --dev >/dev/null
ok=0
for _ in $(seq 1 60); do
  curl -fsS http://127.0.0.1:14445/health/ready >/dev/null 2>&1 && curl -fsS http://127.0.0.1:14444/health/ready >/dev/null 2>&1 && { ok=1; break; }
  sleep 1
done
[ "$ok" = 1 ] || { echo "hydra never became ready"; docker logs "$NAME" 2>&1 | tail -40; exit 1; }

curl -fsS http://127.0.0.1:14444/.well-known/openid-configuration | grep -q '"issuer":"http://127.0.0.1:14444' \
  || { echo "discovery document wrong"; exit 1; }

client="$(curl -fsS -X POST -H 'Content-Type: application/json' http://127.0.0.1:14445/admin/clients \
  -d '{"client_id":"smoke","client_secret":"smoke-secret-0123456789","grant_types":["client_credentials"],"token_endpoint_auth_method":"client_secret_basic"}')"
echo "$client" | grep -q '"client_id":"smoke"' || { echo "client create failed: $client"; exit 1; }

tok="$(curl -fsS -u smoke:smoke-secret-0123456789 -d grant_type=client_credentials http://127.0.0.1:14444/oauth2/token)"
at="$(echo "$tok" | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')"
[ -n "$at" ] || { echo "no access token: $tok"; exit 1; }

curl -fsS -d "token=$at" http://127.0.0.1:14445/admin/oauth2/introspect | grep -q '"active":true' \
  || { echo "token not active on introspection"; exit 1; }
echo "smoke test passed (hydra ${WANT:-?}, uid $user, client_credentials token active)"
