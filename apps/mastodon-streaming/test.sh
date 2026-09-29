#!/usr/bin/env bash
# Smoke test for a built Mastodon streaming image. Usage: test.sh <image-ref> [version]
# Starts it on a read-only rootfs against a throwaway PostgreSQL and Redis and requires the
# health endpoint to answer OK. A missing module, a bad native binding or a Redis or DB
# connection failure all fail this.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
T="mastodon-streaming-smoke-$$"
NET="$T-net"
PORT=14000
cleanup() { docker rm -f "$T" "$T-pg" "$T-redis" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
trap cleanup EXIT
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
docker network create "$NET" >/dev/null
docker run -d --name "$T-pg" --network "$NET" -e POSTGRES_USER=mastodon -e POSTGRES_PASSWORD=pw -e POSTGRES_DB=mastodon_production postgres:17 >/dev/null
docker run -d --name "$T-redis" --network "$NET" redis:7 >/dev/null
for _ in $(seq 1 60); do docker exec "$T-pg" psql -U mastodon -d mastodon_production -c 'select 1' >/dev/null 2>&1 && break; sleep 1; done
docker run -d --name "$T" --network "$NET" --read-only --tmpfs /tmp \
  -e DB_HOST="$T-pg" -e DB_USER=mastodon -e DB_PASS=pw -e DB_NAME=mastodon_production -e REDIS_HOST="$T-redis" \
  -p "127.0.0.1:$PORT:4000" "$IMAGE" >/dev/null
ok=0
for _ in $(seq 1 60); do
  [ "$(curl -s "http://127.0.0.1:$PORT/api/v1/streaming/health" 2>/dev/null)" = "OK" ] && { ok=1; break; }
  sleep 1
done
[ "$ok" = 1 ] || { docker logs "$T" 2>&1 | tail -30; echo "streaming never answered /api/v1/streaming/health"; exit 1; }
# an unauthenticated stream request must be refused by the app itself, not crash it
code="$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$PORT/api/v1/streaming/public")"
[ "$code" = 401 ] || { docker logs "$T" 2>&1 | tail -30; echo "expected 401 without a token, got $code"; exit 1; }
if docker logs "$T" 2>&1 | grep -iE '"level":(50|60)|Error: Cannot find module'; then echo "streaming logged errors"; exit 1; fi
echo "smoke test passed (mastodon-streaming ${2:-?}, uid $user, health OK, auth enforced)"
