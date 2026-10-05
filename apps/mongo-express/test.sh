#!/usr/bin/env bash
# Smoke test for a built mongo-express image. Usage: test.sh <image-ref> [version]
# Runs it on a read-only root against the catalog MongoDB image, writes a document with
# mongosh, and checks the UI shows it behind basic auth, refuses a request without
# credentials, and serves its health check.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
MONGO="${MONGO_IMAGE:-ghcr.io/quenchworks/images/mongodb:8.0.32}"
NAME="mongo-express-smoke-$$"
NET="$NAME-net"
cleanup() { docker rm -f "$NAME" "$NAME-db" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

ver="$(docker run --rm "$IMAGE" -p 'JSON.parse(require("fs").readFileSync("package.json")).version' 2>/dev/null || true)"
[ -n "$ver" ] || ver="$(docker run --rm --entrypoint /usr/bin/node "$IMAGE" -p 'JSON.parse(require("fs").readFileSync("/usr/lib/mongo-express/package.json")).version')"
echo "mongo-express $ver"
[ -z "$WANT" ] || [ "$ver" = "$WANT" ] || { echo "expected version $WANT"; exit 1; }

docker network create "$NET" >/dev/null
docker run -d --name "$NAME-db" --network "$NET" --network-alias mongo --read-only \
  --tmpfs /data:rw,mode=1777,exec -e MONGO_INITDB_ROOT_USERNAME=root \
  -e MONGO_INITDB_ROOT_PASSWORD=rootpw "$MONGO" >/dev/null
ok=0
for _ in $(seq 1 60); do
  docker exec -e HOME=/data/log "$NAME-db" mongosh --quiet -u root -p rootpw \
    --eval 'db.getSiblingDB("quench").items.insertOne({probe: "quench-smoke-doc"})' >/dev/null 2>&1 && { ok=1; break; }
  sleep 2
done
[ "$ok" = 1 ] || { echo "mongodb never accepted the insert"; docker logs "$NAME-db" | tail -20; exit 1; }

docker run -d --name "$NAME" --network "$NET" --read-only --tmpfs /tmp -p 127.0.0.1:18081:8081 \
  -e ME_CONFIG_MONGODB_URL='mongodb://root:rootpw@mongo:27017/?authSource=admin' \
  -e ME_CONFIG_BASICAUTH_ENABLED=true -e ME_CONFIG_BASICAUTH_USERNAME=quench \
  -e ME_CONFIG_BASICAUTH_PASSWORD=smoke-pass -e ME_CONFIG_SITE_SESSIONSECRET=smoke-session \
  -e ME_CONFIG_SITE_COOKIESECRET=smoke-cookie "$IMAGE" >/dev/null
ok=0
for _ in $(seq 1 60); do
  curl -fsS http://127.0.0.1:18081/status >/dev/null 2>&1 && { ok=1; break; }
  sleep 1
done
[ "$ok" = 1 ] || { echo "/status never came up"; docker logs "$NAME" | tail -30; exit 1; }
echo "health check ok"

code="$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:18081/)"
[ "$code" = 401 ] || { echo "expected 401 without credentials, got $code"; exit 1; }
echo "no credentials: 401"

page="$(curl -fsS -u quench:smoke-pass http://127.0.0.1:18081/db/quench/items)"
grep -q 'quench-smoke-doc' <<<"$page" || { echo "document not shown"; docker logs "$NAME" | tail -30; exit 1; }
echo "document visible through the UI"
echo "smoke test passed"
