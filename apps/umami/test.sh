#!/usr/bin/env bash
# Smoke test for a built Umami image. Usage: test.sh <image-ref> [version]
# Starts it on a read-only root against the QuenchWorks PostgreSQL image (start-docker.sh runs
# prisma migrate deploy through the schema engine), logs in as the seeded admin, creates a
# website, sends a page view to /api/send like the tracker does, and reads it back from the
# stats API. A wrong password must be refused.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
TAG="quench-umami-smoke-$$"
NET="$TAG-net"
PG=ghcr.io/quenchworks/images/postgresql@sha256:4c0580fd2968b8d6e8fb008e0eb74b55c54464b041c235842e4d1cc37ed9a445
PW="pg-$$-secret"
URL=http://127.0.0.1:13000

cleanup() { docker rm -f "$TAG-pg" "$TAG-app" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
ver="$(docker run --rm --entrypoint /usr/bin/node "$IMAGE" -p 'require("/app/package.json").version')"
echo "umami $ver"
[ -z "$WANT" ] || [ "$ver" = "$WANT" ] || { echo "expected $WANT"; exit 1; }

docker network create "$NET" >/dev/null
docker run -d --name "$TAG-pg" --network "$NET" --network-alias pg \
  -e POSTGRES_PASSWORD="$PW" -e POSTGRES_DB=umami "$PG" >/dev/null
for i in $(seq 1 60); do
  docker exec "$TAG-pg" pg_isready -h 127.0.0.1 -U postgres -d umami >/dev/null 2>&1 && break
  [ "$i" = 60 ] && { echo "postgres never ready"; docker logs "$TAG-pg" | tail -20; exit 1; }
  sleep 1
done
docker run -d --name "$TAG-app" --network "$NET" -p 127.0.0.1:13000:3000 --read-only --tmpfs /tmp:uid=1001,gid=1001 \
  -e DATABASE_URL="postgresql://postgres:$PW@pg:5432/umami" -e APP_SECRET="smoke-$$-app-secret" "$IMAGE" >/dev/null
for i in $(seq 1 90); do
  grep -q '"ok":true' <<<"$(curl -fsS "$URL/api/heartbeat" 2>/dev/null || true)" && break
  [ "$i" = 90 ] && { echo "/api/heartbeat never ok"; docker logs "$TAG-app" 2>&1 | tail -40; exit 1; }
  sleep 2
done
echo "  migrated and up: /api/heartbeat ok"

bad="$(curl -sS -o /dev/null -w '%{http_code}' -H 'Content-Type: application/json' "$URL/api/auth/login" -d '{"username":"admin","password":"wrong"}')"
[ "$bad" = 401 ] || { echo "a wrong password got HTTP $bad"; exit 1; }
login="$(curl -fsS -H 'Content-Type: application/json' "$URL/api/auth/login" -d '{"username":"admin","password":"umami"}')"
token="$(jq -er '.token' <<<"$login")"
[ -n "$token" ] || { echo "no token: $login"; exit 1; }
echo "  admin login ok, wrong password refused"
auth=(-H "Authorization: Bearer $token" -H 'Content-Type: application/json')
site="$(curl -fsS "${auth[@]}" "$URL/api/websites" -d '{"name":"smoke","domain":"smoke.example"}')"
id="$(jq -er '.id' <<<"$site")"
ua='Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0 Safari/537.36'
curl -fsS -A "$ua" -H 'Content-Type: application/json' "$URL/api/send" \
  -d "{\"type\":\"event\",\"payload\":{\"website\":\"$id\",\"hostname\":\"smoke.example\",\"url\":\"/hello\",\"language\":\"en-US\",\"screen\":\"1920x1080\"}}" >/dev/null
now="$(($(date +%s) * 1000))"
stats=""
for _ in $(seq 1 10); do
  stats="$(curl -fsS "${auth[@]}" "$URL/api/websites/$id/stats?startAt=$((now - 3600000))&endAt=$((now + 60000))")"
  python3 -c 'import json,sys;s=json.loads(sys.argv[1]);v=s["pageviews"];v=v["value"] if isinstance(v,dict) else v;sys.exit(0 if v>=1 else 1)' "$stats" && break
  sleep 1
done
python3 -c 'import json,sys;s=json.loads(sys.argv[1]);v=s["pageviews"];v=v["value"] if isinstance(v,dict) else v;sys.exit(0 if v>=1 else 1)' "$stats" \
  || { echo "page view not counted: $stats"; docker logs "$TAG-app" 2>&1 | tail -20; exit 1; }
echo "  page view sent and counted: $stats"
if docker logs "$TAG-app" 2>&1 | grep -qiE 'read-only file system|EROFS'; then echo "wrote to the root"; docker logs "$TAG-app" 2>&1 | tail -20; exit 1; fi

echo "smoke test passed (umami ${WANT:-?}: migrate on PostgreSQL, login, website, tracked page view in stats, read-only root; user $user)"
