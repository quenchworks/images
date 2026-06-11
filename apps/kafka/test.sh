#!/usr/bin/env bash
# Smoke test for a built kafka image. Usage: test.sh <image-ref>
# Boots a single-node KRaft broker, then round-trips a message through a topic.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-kafka-smoke-$$"
K=/opt/kafka/bin
BS="localhost:9092"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE"
docker run -d --name "$NAME" "$IMAGE" >/dev/null

# KRaft format + broker boot takes a bit; wait for the API to answer
for i in $(seq 1 60); do
  if docker exec "$NAME" "$K/kafka-broker-api-versions.sh" --bootstrap-server "$BS" >/dev/null 2>&1; then
    echo "broker up after ~$((i*2))s"; break
  fi
  [ "$i" = 60 ] && { echo "kafka did not become ready"; docker logs "$NAME" | tail -40; exit 1; }
  sleep 2
done

echo "creating topic"
docker exec "$NAME" "$K/kafka-topics.sh" --bootstrap-server "$BS" \
  --create --topic smoke --partitions 1 --replication-factor 1
docker exec "$NAME" "$K/kafka-topics.sh" --bootstrap-server "$BS" --list | grep -q '^smoke$'

echo "producing a message"
echo "hello-quenchworks" | docker exec -i "$NAME" "$K/kafka-console-producer.sh" \
  --bootstrap-server "$BS" --topic smoke

echo "consuming it back"
docker exec "$NAME" "$K/kafka-console-consumer.sh" --bootstrap-server "$BS" \
  --topic smoke --from-beginning --max-messages 1 --timeout-ms 20000 | grep -q 'hello-quenchworks'

# must run as the nonroot kafka user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "version:"; docker exec "$NAME" "$K/kafka-topics.sh" --version
echo "smoke test passed (nonroot user: $user)"
