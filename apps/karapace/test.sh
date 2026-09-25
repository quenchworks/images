#!/usr/bin/env bash
# Smoke test for a built karapace image. Usage: test.sh <image-ref> [version]
# Real work against a live KRaft broker (the QuenchWorks kafka image): the registry
# stores an Avro and a Protobuf schema (the Protobuf path runs the Go extension), rejects
# an incompatible Protobuf change, and the REST proxy role lists topics.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NET="karapace-smoke-$$"; KAFKA="$NET-kafka"; REG="$NET-reg"; REST="$NET-rest"
cleanup() { docker rm -f "$REG" "$REST" "$KAFKA" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

docker network create "$NET" >/dev/null
docker run -d --name "$KAFKA" --network "$NET" -e KAFKA_ADVERTISED_LISTENERS="PLAINTEXT://$KAFKA:9092" \
  ghcr.io/quenchworks/images/kafka:4.3.1 >/dev/null
for i in $(seq 1 60); do
  docker exec "$KAFKA" /opt/kafka/bin/kafka-broker-api-versions.sh --bootstrap-server localhost:9092 >/dev/null 2>&1 && break
  [ "$i" = 60 ] && { echo "kafka did not become ready"; docker logs "$KAFKA" 2>&1 | tail -30; exit 1; }
  sleep 2
done

docker run -d --name "$REG" --network "$NET" --read-only --tmpfs /tmp \
  -e KARAPACE_BOOTSTRAP_URI="$KAFKA:9092" -e KARAPACE_ADVERTISED_HOSTNAME="$REG" \
  -e KARAPACE_REPLICATION_FACTOR=1 -p 127.0.0.1:18081:8081 "$IMAGE" >/dev/null
R=http://127.0.0.1:18081
ok=0
for _ in $(seq 1 90); do
  curl -fsS "$R/subjects" >/dev/null 2>&1 && { ok=1; break; }
  sleep 1
done
[ "$ok" = 1 ] || { echo "registry never answered"; docker logs "$REG" 2>&1 | tail -40; exit 1; }

H='Content-Type: application/vnd.schemaregistry.v1+json'
avro='{"schema":"{\"type\":\"record\",\"name\":\"Order\",\"fields\":[{\"name\":\"id\",\"type\":\"string\"}]}"}'
# the registry answers reads before it has elected itself primary and read _schemas;
# writes return 500 until then, so the first registration retries
ok=0
for _ in $(seq 1 60); do
  curl -fsS -X POST -H "$H" -d "$avro" "$R/subjects/orders-value/versions" 2>/dev/null | grep -q '"id"' && { ok=1; break; }
  sleep 1
done
[ "$ok" = 1 ] || { echo "avro register failed"; curl -sS -X POST -H "$H" -d "$avro" "$R/subjects/orders-value/versions" || true; docker logs "$REG" 2>&1 | tail -40; exit 1; }
proto='{"schemaType":"PROTOBUF","schema":"syntax = \"proto3\"; package q; message Order { string id = 1; int64 cents = 2; }"}'
curl -fsS -X POST -H "$H" -d "$proto" "$R/subjects/orders-proto-value/versions" | grep -q '"id"' || { echo "protobuf register failed"; docker logs "$REG" 2>&1 | tail -40; exit 1; }
bad='{"schemaType":"PROTOBUF","schema":"syntax = \"proto3\"; package q; message Order { int32 id = 1; }"}'
curl -fsS -X POST -H "$H" -d "$bad" "$R/compatibility/subjects/orders-proto-value/versions/latest" | tee /tmp/compat.$$ | grep -q '"is_compatible":false' \
  || { echo "incompatible protobuf change not rejected: $(cat /tmp/compat.$$)"; rm -f /tmp/compat.$$; exit 1; }
rm -f /tmp/compat.$$
curl -fsS "$R/subjects" | grep -q 'orders-proto-value' || { echo "subjects missing"; exit 1; }

docker run -d --name "$REST" --network "$NET" --read-only --tmpfs /tmp \
  -e KARAPACE_BOOTSTRAP_URI="$KAFKA:9092" -e KARAPACE_KARAPACE_REST=true -e KARAPACE_KARAPACE_REGISTRY=false \
  -e KARAPACE_PORT=8082 -e KARAPACE_REGISTRY_HOST="$REG" -e KARAPACE_REGISTRY_PORT=8081 \
  -p 127.0.0.1:18082:8082 --entrypoint /opt/karapace/venv/bin/python "$IMAGE" -m karapace.kafka_rest_apis >/dev/null
ok=0
for _ in $(seq 1 60); do
  curl -fsS http://127.0.0.1:18082/topics 2>/dev/null | grep -q '_schemas' && { ok=1; break; }
  sleep 1
done
[ "$ok" = 1 ] || { echo "rest proxy did not list topics"; docker logs "$REST" 2>&1 | tail -30; exit 1; }
echo "smoke test passed (karapace ${WANT:-?}, uid $user, avro + protobuf registry, compatibility, rest proxy)"
