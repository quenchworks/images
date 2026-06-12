#!/usr/bin/env bash
# Smoke test for a built CouchDB image. Usage: test.sh <image-ref>
# Runs the image with a READ-ONLY rootfs + writable tmpfs (mode=1777), then exercises
# the HTTP API: welcome, _up, admin auth, db + doc roundtrip, and a QuickJS map view
# (the view proves the bundled JS engine linked and runs). Confirms nonroot uid 1001.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-couchdb-smoke-$$"
USER_NAME="admin"
PASS="s3cret-couch"
PORT=15984

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE (read-only rootfs + tmpfs state)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /var/lib/couchdb:rw,mode=1777 \
  --tmpfs /tmp:rw,mode=1777 \
  -e COUCHDB_USER="$USER_NAME" \
  -e COUCHDB_PASSWORD="$PASS" \
  -e COUCHDB_ERLANG_COOKIE="smoke-cookie" \
  -p "${PORT}:5984" \
  "$IMAGE" >/dev/null

base="http://127.0.0.1:${PORT}"
auth="-u ${USER_NAME}:${PASS}"

# Wait for the HTTP API to come up.
ready=0
for i in $(seq 1 90); do
  if curl -fsS "${base}/" >/dev/null 2>&1; then ready=1; echo "API up after ~${i}s"; break; fi
  if ! docker ps -q --filter "name=${NAME}" | grep -q .; then
    echo "container exited early"; docker logs "$NAME" | tail -40; exit 1
  fi
  sleep 1
done
[ "$ready" = 1 ] || { echo "couchdb did not become ready"; docker logs "$NAME" | tail -40; exit 1; }

# nonroot uid 1001
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "1) welcome JSON"
welcome="$(curl -fsS "${base}/")"
echo "$welcome" | grep -q '"couchdb":"Welcome"' || { echo "no welcome: $welcome"; exit 1; }

# Single-node setup: create the system databases (_users/_replicator/_global_changes).
echo "2) single-node setup (system DBs)"
for db in _users _replicator _global_changes; do
  code="$(curl -fsS -o /dev/null -w '%{http_code}' $auth -X PUT "${base}/${db}" || true)"
  case "$code" in 201|202|412) ;; *) echo "system db ${db} PUT -> $code"; exit 1;; esac
done

echo "3) _up status"
up="$(curl -fsS $auth "${base}/_up")"
echo "$up" | grep -q '"status":"ok"' || { echo "_up not ok: $up"; exit 1; }

echo "4) db + doc roundtrip"
curl -fsS $auth -X PUT "${base}/testdb" >/dev/null
curl -fsS $auth -X PUT "${base}/testdb/doc1" \
  -H 'Content-Type: application/json' \
  -d '{"name":"quench","n":42}' >/dev/null
got="$(curl -fsS $auth "${base}/testdb/doc1")"
echo "$got" | grep -q '"name":"quench"' || { echo "doc roundtrip failed: $got"; exit 1; }

# Add a few more docs so the view has something to emit.
for n in 1 2 3; do
  curl -fsS $auth -X PUT "${base}/testdb/k${n}" \
    -H 'Content-Type: application/json' \
    -d "{\"v\":${n}}" >/dev/null
done

echo "5) QuickJS map view (proves the JS engine)"
# A design doc with a JS map function. Querying it forces CouchDB to spin up the
# QuickJS view server (couchjs_mainjs) and run the map -- if the engine were broken
# the view query would 500 / time out.
curl -fsS $auth -X PUT "${base}/testdb/_design/d" \
  -H 'Content-Type: application/json' \
  -d '{"views":{"byv":{"map":"function(doc){ if(doc.v){ emit(doc.v, doc.v*2); } }"}}}' >/dev/null

view="$(curl -fsS $auth "${base}/testdb/_design/d/_view/byv")"
echo "view result: $view"
echo "$view" | grep -q '"key":1,"value":2' || { echo "JS view did not return expected rows: $view"; docker logs "$NAME" | tail -30; exit 1; }

echo "version:"; curl -fsS "${base}/" | grep -o '"version":"[^"]*"'
echo "smoke test passed (nonroot user: $user, QuickJS view server OK)"
