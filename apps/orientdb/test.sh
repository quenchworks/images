#!/usr/bin/env bash
# Smoke test for the OrientDB image. Usage: test.sh <image-ref> [version]
# Boots the server, creates a database over the HTTP API (2480), runs an SQL insert
# and select, restarts the container and reads the record back.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
NAME="quench-orientdb-smoke-$$"
PASS="quenchtest123"
API=http://127.0.0.1:2480
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

docker run -d --name "$NAME" -e ORIENTDB_ROOT_PASSWORD="$PASS" \
  -p 127.0.0.1:2480:2480 -p 127.0.0.1:2424:2424 "$IMAGE" >/dev/null

wait_up() {
  for i in $(seq 1 90); do
    code="$(curl -s -o /dev/null -w '%{http_code}' -u "root:$PASS" "$API/listDatabases" || true)"
    [ "$code" = 200 ] && return 0
    sleep 2
  done
  echo "orientdb did not become ready"; docker logs "$NAME" 2>&1 | tail -60; exit 1
}
sql() { curl -fsS -u "root:$PASS" -H 'Content-Type: application/json' \
  -d "{\"command\":\"$1\"}" "$API/command/smoke/sql"; }

wait_up
logs="$(docker logs "$NAME" 2>&1)"
if grep -qi 'hazelcast' <<<"$logs"; then echo "hazelcast referenced at boot"; echo "$logs" | tail -40; exit 1; fi

curl -fsS -u "root:$PASS" -X POST "$API/database/smoke/plocal" >/dev/null \
  || { echo "database create failed"; docker logs "$NAME" 2>&1 | tail -40; exit 1; }
sql "CREATE CLASS Thing" >/dev/null
sql "INSERT INTO Thing SET name = 'quench'" >/dev/null
out="$(sql "SELECT name FROM Thing")"
grep -q '"quench"' <<<"$out" || { echo "select did not return the record: $out"; exit 1; }
echo "SQL round-trip ok: ${out:0:60}"

# wrong password must be refused
code="$(curl -s -o /dev/null -w '%{http_code}' -u root:wrong "$API/database/smoke")"
[ "$code" = 401 ] || { echo "expected 401 for a wrong password, got $code"; exit 1; }

docker restart "$NAME" >/dev/null
wait_up
out="$(sql "SELECT name FROM Thing")"
grep -q '"quench"' <<<"$out" || { echo "record lost across restart: $out"; exit 1; }
echo "record survived restart; smoke test passed (user $user)"
