#!/usr/bin/env bash
# Smoke test for a built spark image. Usage: test.sh <image-ref> [version]
# Runs real work, not a banner: spark-submit reports the version, SparkPi computes
# in local mode on a read-only rootfs, and a standalone master registers a worker.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NET="spark-smoke-$$"
RUN=(docker run --rm --read-only --tmpfs /tmp --tmpfs /opt/spark/work-dir:uid=1001,gid=1001)

cleanup() { docker rm -f "$NET-master" "$NET-worker" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "== spark-submit --version =="
ver="$("${RUN[@]}" "$IMAGE" /opt/spark/bin/spark-submit --version 2>&1)"
echo "$ver" | grep -E 'version [0-9]' | head -2
if [ -n "$WANT" ]; then
  echo "$ver" | grep -q "version $WANT" || { echo "expected version $WANT"; exit 1; }
fi

echo "== SparkPi in local mode =="
pi="$("${RUN[@]}" "$IMAGE" /opt/spark/bin/spark-submit --master 'local[2]' \
  --class org.apache.spark.examples.SparkPi /opt/spark/examples/jars/spark-examples.jar 20 2>&1)"
echo "$pi" | grep 'Pi is roughly' || { echo "$pi" | tail -40; exit 1; }

echo "== runs as uid 1001 =="
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected configured user 1001, got '$user'"; exit 1; }

echo "== standalone master registers a worker =="
docker network create "$NET" >/dev/null
docker run -d --name "$NET-master" --network "$NET" --hostname master "$IMAGE" \
  /opt/spark/bin/spark-class org.apache.spark.deploy.master.Master --host master --port 7077 --webui-port 8080 >/dev/null
docker run -d --name "$NET-worker" --network "$NET" -e SPARK_WORKER_DIR=/tmp/spark-worker "$IMAGE" \
  /opt/spark/bin/spark-class org.apache.spark.deploy.worker.Worker spark://master:7077 --cores 1 --memory 512m >/dev/null
for i in $(seq 1 60); do
  docker logs "$NET-master" 2>&1 | grep -q 'Registering worker' && break
  sleep 2
done
docker logs "$NET-master" 2>&1 | grep 'Registering worker' || { docker logs "$NET-master" 2>&1 | tail -40; docker logs "$NET-worker" 2>&1 | tail -40; exit 1; }

echo "smoke test passed (Spark ${WANT:-?}, uid $user)"
