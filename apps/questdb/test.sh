#!/usr/bin/env bash
# Smoke test for a built questdb image. Usage: test.sh <image-ref> [expected-version]
# Boots QuestDB, waits for the :9000 REST API, then round-trips a table through
# the SQL engine and checks the reported build version.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
WANT="${2:-}"
NAME="quench-questdb-smoke-$$"
PORT=19000
BASE="http://127.0.0.1:${PORT}"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

q() { # url-encoded SQL -> REST /exec
  curl -fsS --get "$BASE/exec" --data-urlencode "query=$1"
}

echo "starting $IMAGE"
docker run -d --name "$NAME" -p "${PORT}:9000" "$IMAGE" >/dev/null

# HTTP server + SQL engine boot takes a bit; wait for the REST API to answer
for i in $(seq 1 60); do
  if q "SELECT 1" >/dev/null 2>&1; then
    echo "questdb up after ~$((i*2))s"; break
  fi
  [ "$i" = 60 ] && { echo "questdb did not become ready"; docker logs "$NAME" | tail -40; exit 1; }
  sleep 2
done

echo "creating a table + inserting a row"
q "CREATE TABLE smoke(ts TIMESTAMP, v DOUBLE) TIMESTAMP(ts) PARTITION BY DAY" >/dev/null
q "INSERT INTO smoke VALUES(now(), 0.64)" >/dev/null

echo "reading it back"
# WAL tables apply writes asynchronously, so the row can lag the INSERT ack by a
# beat -- poll the count instead of reading once.
for i in $(seq 1 20); do
  q "SELECT count() FROM smoke" | grep -q '\[\[1\]\]' && { echo "row visible"; break; }
  [ "$i" = 20 ] && { echo "expected 1 row"; q "SELECT count() FROM smoke"; exit 1; }
  sleep 1
done

echo "build version:"
build="$(q "SELECT build()")"
echo "  $build"
if [ -n "$WANT" ]; then
  echo "$build" | grep -q "$WANT" || { echo "expected version $WANT in build()"; exit 1; }
fi

# must run as the nonroot questdb user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
