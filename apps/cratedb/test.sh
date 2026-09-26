#!/usr/bin/env bash
# Smoke test for a built CrateDB image. Usage: test.sh <image-ref> [version]
# Single node: reads the version, writes and queries a table over the HTTP SQL
# endpoint, and requires the rows to survive a restart.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="quench-cratedb-smoke-$$"
URL=http://127.0.0.1:14200

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

# host-based auth only trusts _local_ by default and the test calls from the host
docker run -d --name "$NAME" -p 127.0.0.1:14200:4200 "$IMAGE" \
  -Cpath.data=/data -Cnetwork.host=_site_ -Cdiscovery.type=single-node -Cauth.host_based.enabled=false >/dev/null

wait_up() {
  for i in $(seq 1 90); do
    curl -fsS "$URL/" >/dev/null 2>&1 && return 0
    sleep 2
  done
  echo "CrateDB never answered"; docker logs "$NAME" | tail -40; exit 1
}
sql() { curl -fsS -H 'Content-Type: application/json' -d "{\"stmt\":\"$1\"}" "$URL/_sql"; }
wait_up

ver="$(curl -fsS "$URL/" | sed -n 's/.*"number" *: *"\([^"]*\)".*/\1/p')"
echo "reported version: $ver"
[ -n "$ver" ] || { echo "no version"; exit 1; }
[ -z "$WANT" ] || [ "$ver" = "$WANT" ] || { echo "expected $WANT"; exit 1; }

sql "CREATE TABLE smoke (id INT PRIMARY KEY, v TEXT)" >/dev/null
sql "INSERT INTO smoke (id, v) VALUES (1, 'quench'), (2, 'works')" >/dev/null
sql "REFRESH TABLE smoke" >/dev/null
out="$(sql "SELECT count(*) FROM smoke")"
grep -q '"rows":\[\[2\]\]' <<<"$out" || { echo "unexpected count: $out"; exit 1; }
echo "  SQL over HTTP: created, inserted, counted 2 rows"

docker restart "$NAME" >/dev/null
wait_up
for i in $(seq 1 30); do
  out="$(sql "SELECT v FROM smoke WHERE id = 2" 2>/dev/null)" && grep -q '"works"' <<<"$out" && break
  [ "$i" = 30 ] && { echo "rows lost across a restart: $out"; exit 1; }
  sleep 2
done
echo "  rows kept across a restart"

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (version $ver, nonroot user: $user, SQL write/read, restart)"
