#!/usr/bin/env bash
# Smoke test for a built akhq image. Usage: test.sh <image-ref> [version]
# Real Kafka work: a single-node KRaft broker (the QuenchWorks kafka image) gets a
# topic, and AKHQ, started on a read-only root, must report healthy, serve the UI and
# list that topic through its API.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NET="akhq-smoke-$$"; KAFKA="$NET-kafka"; NAME="$NET-akhq"
cleanup() { docker rm -f "$NAME" "$KAFKA" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

docker network create "$NET" >/dev/null
docker run -d --name "$KAFKA" --network "$NET" -e KAFKA_ADVERTISED_LISTENERS="PLAINTEXT://$KAFKA:9092" \
  ghcr.io/quenchworks/images/kafka:4.3.1 >/dev/null
K=/opt/kafka/bin
for i in $(seq 1 60); do
  docker exec "$KAFKA" "$K/kafka-broker-api-versions.sh" --bootstrap-server localhost:9092 >/dev/null 2>&1 && break
  [ "$i" = 60 ] && { echo "kafka did not become ready"; docker logs "$KAFKA" 2>&1 | tail -30; exit 1; }
  sleep 2
done
docker exec "$KAFKA" "$K/kafka-topics.sh" --bootstrap-server localhost:9092 --create --topic quench-smoke --partitions 1 --replication-factor 1 >/dev/null

WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
trap 'rm -rf "$WORK"; cleanup' EXIT
cat > "$WORK/application.yml" <<CONF
micronaut:
  server:
    port: 8080
endpoints:
  all:
    port: 28081
akhq:
  connections:
    smoke:
      properties:
        bootstrap.servers: "$KAFKA:9092"
CONF
chmod -R a+rX "$WORK"
docker run -d --name "$NAME" --network "$NET" --read-only --tmpfs /tmp \
  -v "$WORK/application.yml:/etc/akhq/application.yml:ro" \
  -p 127.0.0.1:18080:8080 -p 127.0.0.1:28081:28081 "$IMAGE" >/dev/null
ok=0
for _ in $(seq 1 90); do
  curl -fsS http://127.0.0.1:28081/health 2>/dev/null | grep -q '"status":"UP"' && { ok=1; break; }
  sleep 1
done
[ "$ok" = 1 ] || { echo "akhq never reported UP"; curl -sS http://127.0.0.1:28081/health || true; docker logs "$NAME" 2>&1 | tail -40; exit 1; }

curl -fsS -o /dev/null http://127.0.0.1:18080/ui || { echo "UI not served"; exit 1; }
topics="$(curl -fsS 'http://127.0.0.1:18080/api/smoke/topic?show=ALL')"
echo "$topics" | grep -q '"name":"quench-smoke"' || { echo "topic not listed: $topics" | head -c 600; exit 1; }
if docker logs "$NAME" 2>&1 | grep -E ' ERROR |Exception'; then echo "akhq logged errors"; exit 1; fi
echo "smoke test passed (akhq ${WANT:-?}, uid $user, topic listed from a live broker)"
