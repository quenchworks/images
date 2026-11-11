#!/usr/bin/env bash
# Smoke test for a built apache-pinot image. Usage: test.sh <image-ref> [version]
#
# Runs Pinot's own batch QuickStart in one container, as uid 1001 on a read-only rootfs (tmpfs
# /tmp): it starts ZooKeeper, a controller, a broker, servers and a minion, creates the example
# tables, builds and uploads segments, then the test queries the broker over HTTP and checks
# real rows come back (airlineStats, built from Avro by a batch ingestion job). QuickStart writes
# segments next to its examples, so they are copied to a writable dir first; the image has no
# wget, so the host queries a published port.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="pinot-smoke-$$"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

if [ -n "$WANT" ]; then
  jar="$(docker run --rm --entrypoint /bin/sh "$IMAGE" -c 'ls /opt/pinot/lib/')"
  grep -q "pinot-all-${WANT}-jar-with-dependencies.jar" <<<"$jar" || { echo "expected pinot $WANT: $jar"; exit 1; }
fi

PORT="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])')"
docker run -d --name "$NAME" --read-only --tmpfs /tmp:rw,exec,size=3g --memory 5g \
  -e JAVA_OPTS="-Xms512m -Xmx2g" -e HOME=/tmp -p "127.0.0.1:${PORT}:8000" --entrypoint /bin/sh "$IMAGE" \
  -c 'mkdir -p /tmp/w && cp -r /opt/pinot/examples /tmp/w/ && cd /tmp/w && exec /opt/pinot/bin/pinot-admin.sh QuickStart -type batch -dataDir /tmp/qs' >/dev/null

q='{"sql":"select count(*) from airlineStats"}'
out=""
for i in $(seq 1 240); do
  out="$(curl -s -m 5 -H 'Content-Type: application/json' -d "$q" "http://127.0.0.1:${PORT}/query/sql" || true)"
  grep -q '"rows":\[\[[1-9]' <<<"$out" && break
  docker ps --filter "name=$NAME" --filter status=running --format '{{.Names}}' | grep -q "$NAME" \
    || { echo "container died:"; docker logs "$NAME" 2>&1 | tail -60; exit 1; }
  [ "$i" = 240 ] && { echo "no rows from the broker: ${out:0:300}"; docker logs "$NAME" 2>&1 | tail -60; exit 1; }
  sleep 2
done
echo "broker answered: $(grep -oE '"rows":\[\[[0-9]+' <<<"$out")"

if docker logs "$NAME" 2>&1 | grep -qiE 'read-only file system|NoClassDefFoundError|NoSuchMethodError|OutOfMemoryError'; then
  echo "errors in the logs:"; docker logs "$NAME" 2>&1 | grep -iE 'read-only file system|NoClassDefFoundError|NoSuchMethodError|OutOfMemoryError' | head; exit 1
fi
echo "smoke test passed (nonroot $user, read-only rootfs, QuickStart batch: zookeeper, controller, broker, server, query)"
