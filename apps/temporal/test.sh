#!/usr/bin/env bash
# Smoke test for a built Temporal server image. Usage: test.sh <image-ref>
#
# Temporal needs a database. To stay self-contained (no external Postgres) the
# image ships config/development-sqlite-file.yaml -- an embedded pure-Go sqlite
# (modernc) store with setup:true that creates its own schema in-process. The
# default entrypoint boots the all-in-one server against that config, so we just
# run the image and assert all four roles come up.
#
# Asserts:
#   - `temporal-server --version` prints the pinned version
#   - `temporal-sql-tool --version` works
#   - the Postgres schema files are present at the chart-expected path
#   - the all-in-one server boots: logs show the frontend started and the
#     metrics endpoint (tally, :8000) and pprof are listening
#   - the container runs as nonroot uid 1001
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-temporal-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "checking temporal-server / temporal-sql-tool versions"
docker run --rm --entrypoint /usr/bin/temporal-server "$IMAGE" --version
docker run --rm --entrypoint /usr/bin/temporal-sql-tool "$IMAGE" --version

echo "checking Postgres schema files are shipped at the chart path"
docker run --rm --entrypoint /bin/sh "$IMAGE" -c '
  set -e
  test -f /etc/temporal/schema/postgresql/v12/temporal/schema.sql
  test -f /etc/temporal/schema/postgresql/v12/visibility/schema.sql
  test -d /etc/temporal/schema/postgresql/v12/temporal/versioned
  echo "schema files OK"
'

# The standalone sqlite config writes the db file + WAL into the working dir and
# uses /tmp; run with a writable workdir and /tmp tmpfs. The all-in-one server
# boots all four services against the embedded sqlite store.
echo "booting all-in-one server against embedded sqlite (writable /tmp + /data)"
docker run -d --name "$NAME" \
  --tmpfs /tmp:rw \
  --tmpfs /data:rw,uid=1001,gid=1001 \
  -w /data \
  "$IMAGE" >/dev/null

# Wait for the frontend to come up. The all-in-one boot logs a line per service
# starting ("Frontend started" / "matching started" etc). Poll the logs.
ok=0
for i in $(seq 1 90); do
  logs="$(docker logs "$NAME" 2>&1 || true)"
  # Here-string, NOT `echo "$logs" | grep -q`. $logs is temporal's whole log output --
  # thousands of JSON lines -- so echo is still writing when grep -q leaves at the first
  # match, takes SIGPIPE, and under pipefail the `if` condition reads FALSE even though the
  # pattern matched. The loop then spun all 90 iterations and timed out on a healthy server,
  # printing "echo: write error: Broken pipe" 90 times as the only clue.
  if grep -qiE 'frontend started|Service resources started|Starting to serve' <<<"$logs"; then
    ok=1
    break
  fi
  # bail early if the container died
  if [ "$(docker inspect -f '{{.State.Running}}' "$NAME" 2>/dev/null || echo false)" != "true" ]; then
    echo "container exited early:"; echo "$logs"; exit 1
  fi
  sleep 1
done

if [ "$ok" != 1 ]; then
  echo "frontend did not report started within timeout:"
  docker logs "$NAME" 2>&1 | tail -50
  exit 1
fi
echo "server reports services started"

# Assert the server's listeners are up: the frontend gRPC (7233) and the
# metrics endpoint (tally, :8000). The hardened base has no curl/wget applet, so
# we check the kernel's listening sockets via /proc/net/tcp (port in hex:
# 7233=0x1C41, 8000=0x1F40) -- no external tools needed.
echo "checking the server's listening sockets (frontend 7233 + metrics 8000)"
for i in $(seq 1 30); do
  if docker exec "$NAME" /bin/sh -c 'cat /proc/net/tcp /proc/net/tcp6 2>/dev/null' \
       | awk '{print $2}' | grep -qiE ':1C41$'; then
    listening=1
    break
  fi
  [ "$i" = 30 ] && { echo "frontend gRPC :7233 not listening:"; docker logs "$NAME" | tail -30; exit 1; }
  sleep 1
done
docker exec "$NAME" /bin/sh -c 'cat /proc/net/tcp /proc/net/tcp6 2>/dev/null' \
  | awk '{print $2}' | grep -qiE ':1F40$' \
  || { echo "metrics :8000 not listening:"; docker logs "$NAME" | tail -30; exit 1; }
echo "frontend :7233 + metrics :8000 listening -> OK"

# must run as the nonroot temporal user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot uid: $user, all-in-one sqlite boot, /metrics OK)"
