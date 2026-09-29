#!/usr/bin/env bash
# Smoke test for a built Mastodon image. Usage: test.sh <image-ref> [version]
# Boots against a throwaway PostgreSQL and Redis on a read-only rootfs: runs the schema
# setup through the image's own launcher, then requires puma to answer /health and render
# the about page, sidekiq to start and connect, and tootctl to report the version. This
# proves the gems, libvips, the database driver and the compiled assets, not just a port.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
T="mastodon-smoke-$$"
NET="$T-net"
PORT=13000
cleanup() { docker rm -f "$T-web" "$T-sk" "$T-pg" "$T-redis" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
trap cleanup EXIT
fail() { echo "$1"; for c in web sk; do echo "--- $c"; docker logs "$T-$c" 2>&1 | tail -30; done; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

docker network create "$NET" >/dev/null
docker run -d --name "$T-pg" --network "$NET" -e POSTGRES_USER=mastodon -e POSTGRES_PASSWORD=pw -e POSTGRES_DB=mastodon_production postgres:17 >/dev/null
docker run -d --name "$T-redis" --network "$NET" redis:7 >/dev/null
for _ in $(seq 1 60); do docker exec "$T-pg" psql -U mastodon -d mastodon_production -c 'select 1' >/dev/null 2>&1 && break; sleep 1; done

env_args=(-e LOCAL_DOMAIN=smoke.example.com -e DB_HOST="$T-pg" -e DB_USER=mastodon -e DB_PASS=pw -e DB_NAME=mastodon_production
  -e REDIS_HOST="$T-redis" -e SECRET_KEY_BASE="$(head -c 64 /dev/urandom | od -An -tx1 | tr -d ' \n')"
  -e OTP_SECRET="$(head -c 64 /dev/urandom | od -An -tx1 | tr -d ' \n')"
  -e ACTIVE_RECORD_ENCRYPTION_DETERMINISTIC_KEY=smokedeterministickeysmokedeterm
  -e ACTIVE_RECORD_ENCRYPTION_KEY_DERIVATION_SALT=smokesaltsmokesaltsmokesaltsmoke
  -e ACTIVE_RECORD_ENCRYPTION_PRIMARY_KEY=smokeprimarykeysmokeprimarykeysm)
ro=(--read-only --tmpfs /tmp:mode=1777,exec --tmpfs /opt/mastodon/public/system:uid=1001,gid=1001)

docker run --rm --network "$NET" "${ro[@]}" "${env_args[@]}" -e SAFETY_ASSURED=1 "$IMAGE" rails db:setup > /tmp/$T-setup.log 2>&1 \
  || { tail -30 /tmp/$T-setup.log; echo "rails db:setup failed"; exit 1; }
echo "schema loaded"

docker run -d --name "$T-web" --network "$NET" "${ro[@]}" "${env_args[@]}" -p "127.0.0.1:$PORT:3000" "$IMAGE" web >/dev/null
docker run -d --name "$T-sk" --network "$NET" "${ro[@]}" "${env_args[@]}" "$IMAGE" sidekiq >/dev/null
ok=0
for _ in $(seq 1 90); do
  [ "$(curl -s -o /dev/null -w '%{http_code}' -H 'Host: smoke.example.com' -H 'X-Forwarded-Proto: https' "http://127.0.0.1:$PORT/health")" = 200 ] && { ok=1; break; }
  sleep 1
done
[ "$ok" = 1 ] || fail "puma never answered /health"

about="$(curl -sS -H 'Host: smoke.example.com' -H 'X-Forwarded-Proto: https' "http://127.0.0.1:$PORT/about")"
grep -q '/packs/' <<<"$about" || fail "about page does not reference the compiled packs"
inst="$(curl -fsS -H 'Host: smoke.example.com' -H 'X-Forwarded-Proto: https' "http://127.0.0.1:$PORT/api/v2/instance")"
grep -q '"domain":"smoke.example.com"' <<<"$inst" || fail "instance API did not answer for the local domain"
ver="$(sed -n 's/.*"version":"\([^"]*\)".*/\1/p' <<<"$inst")"
echo "instance version: $ver"
[ -z "$WANT" ] || [ "$ver" = "$WANT" ] || fail "expected $WANT, instance reports $ver"

for _ in $(seq 1 60); do docker logs "$T-sk" 2>&1 | grep -q 'Booted Rails\|Starting processing' && break; sleep 1; done
docker logs "$T-sk" 2>&1 | grep -q 'Booted Rails\|Starting processing' || fail "sidekiq did not start"
docker run --rm "${ro[@]}" "${env_args[@]}" --network "$NET" "$IMAGE" rails runner 'require "vips"; puts "vips #{Vips.version_string}"' 2>&1 | grep -q '^vips 8\.' || fail "libvips does not load"
echo "smoke test passed (mastodon ${WANT:-?}, uid $user, schema, web, sidekiq, packs, libvips)"
