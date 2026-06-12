#!/usr/bin/env bash
# Smoke test for the standalone DocumentDB image. Usage: test.sh <image-ref>
# Verifies the MongoDB wire protocol end to end: connect through the built-in
# gateway, insert a document, read it back. The image has no shell mongo client,
# so we drive it from a throwaway mongosh container on a shared docker network.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NET="quench-ddb-net-$$"
NAME="quench-ddb-smoke-$$"
PASS="quenchpw1"
USER="default_user"
MONGOSH_IMAGE="mongodb/mongodb-community-server:latest"

cleanup() {
  docker rm -f "$NAME" >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker network create "$NET" >/dev/null

echo "starting $IMAGE"
docker run -d --name "$NAME" --network "$NET" \
  -e DOCUMENTDB_PASSWORD="$PASS" \
  -p 127.0.0.1:10260:10260 "$IMAGE" >/dev/null

# First boot runs initdb + CREATE EXTENSION + create_user, then starts PG and the
# gateway. Wait for the wire-protocol port to accept connections.
echo "waiting for the gateway to accept connections on :10260"
for i in $(seq 1 90); do
  if docker run --rm --network "$NET" busybox sh -c "nc -z $NAME 10260" 2>/dev/null; then
    break
  fi
  [ "$i" = 90 ] && { echo "gateway did not open :10260"; docker logs "$NAME" | tail -60; exit 1; }
  sleep 2
done
# give the gateway a moment to finish TLS/cert generation
sleep 3

URI="mongodb://${USER}:${PASS}@${NAME}:10260/?tls=true&tlsAllowInvalidCertificates=true&authMechanism=SCRAM-SHA-256"

run_mongosh() {
  docker run --rm --network "$NET" "$MONGOSH_IMAGE" \
    mongosh "$URI" --quiet --eval "$1"
}

echo "ping through the gateway"
for i in $(seq 1 30); do
  if run_mongosh 'db.runCommand({ping:1}).ok' 2>/dev/null | grep -q 1; then
    break
  fi
  [ "$i" = 30 ] && { echo "ping failed"; docker logs "$NAME" | tail -60; exit 1; }
  sleep 2
done

echo "insert + find a document over the MongoDB wire protocol"
out="$(run_mongosh '
  const c = db.getSiblingDB("smoke").coll;
  c.insertOne({_id: 1, hello: "quench"});
  print(c.findOne({_id: 1}).hello);
' 2>/dev/null || true)"
echo "  gateway responded: ${out:-<none>}"
echo "$out" | grep -q "quench" || { echo "insert/find roundtrip failed"; docker logs "$NAME" | tail -60; exit 1; }

# must run as the nonroot documentdb user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user, mongo wire protocol verified)"
