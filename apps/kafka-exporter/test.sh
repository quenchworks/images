#!/usr/bin/env bash
# Smoke test for a built kafka-exporter image. Usage: test.sh <image-ref> [expected-version]
#
# Starts the catalog's Kafka image (single-node KRaft on localhost:9092) and runs the
# exporter in the broker's network namespace, so both see localhost. The exporter must
# report one broker and the partitions of a topic the test creates.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
WANT="${2:-}"
KAFKA="${KAFKA_IMAGE:-ghcr.io/quenchworks/images/kafka:4.3.1}"
KN="quench-kafkaexp-broker-$$"; EN="quench-kafkaexp-$$"
cleanup() { docker rm -f "$EN" "$KN" >/dev/null 2>&1 || true; }
trap cleanup EXIT
fail() { echo "FAIL: $*"; docker logs "$EN" 2>&1 | tail -20; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
ver="$(docker run --rm "$IMAGE" --version 2>&1 | head -1)"
echo "$ver"
[ -z "$WANT" ] || grep -q "version $WANT" <<<"$ver" || { echo "expected version $WANT"; exit 1; }

docker run -d --name "$KN" -p 127.0.0.1:19308:9308 "$KAFKA" >/dev/null
for i in $(seq 1 60); do
  logs="$(docker logs "$KN" 2>&1)"
  grep -q 'Kafka Server started' <<<"$logs" && break
  [ "$i" = 60 ] && { echo "$logs" | tail -20; echo "FAIL: broker never started"; exit 1; }
  sleep 2
done
docker exec "$KN" /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 \
  --create --topic quench-smoke --partitions 2 >/dev/null

docker run -d --name "$EN" --network "container:$KN" "$IMAGE" --kafka.server=localhost:9092 >/dev/null
for i in $(seq 1 30); do
  body="$(curl -fsS --max-time 2 http://127.0.0.1:19308/metrics 2>/dev/null || true)"
  grep -q '^kafka_brokers 1$' <<<"$body" && break
  [ "$(docker inspect -f '{{.State.Status}}' "$EN")" = running ] || fail "exporter exited"
  sleep 1
done
grep -q '^kafka_brokers 1$' <<<"$body" || fail "kafka_brokers is not 1"
grep -q '^kafka_topic_partitions{topic="quench-smoke"} 2$' <<<"$body" || fail "topic quench-smoke with 2 partitions not reported"
echo "smoke test passed (uid $user, 1 broker, topic partitions scraped)"
