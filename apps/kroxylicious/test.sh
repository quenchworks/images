#!/usr/bin/env bash
# Smoke test for a built Kroxylicious image. Usage: test.sh <image-ref> [version]
#
# An open port proves nothing, so this test needs Kafka traffic through the proxy:
#   1. `--version` reports the version.
#   2. The image boots on a READ-ONLY rootfs (tmpfs /tmp, where the JNI libraries extract)
#      as uid 1001 and answers /livez on the management port.
#   3. Kafka through the proxy: a broker from ghcr.io/quenchworks/images/kafka, Kroxylicious
#      in front of it, and the broker's own CLI creates a topic, produces and consumes a
#      message with only the proxy's address as bootstrap. Every Kafka request (metadata,
#      create, produce, fetch) goes through Kroxylicious, which rewrites broker addresses to
#      its own.
# The config goes in with `docker cp` (a bind mount of /tmp is refused under Docker
# Desktop, and docker cp is refused on a read-only rootfs), so step 3 runs writable.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
KAFKA_IMAGE="${KAFKA_IMAGE:-ghcr.io/quenchworks/images/kafka:4.3.2}"
TAG="quench-krox-smoke-$$"
NET="$TAG-net"
KAFKA="$TAG-kafka"
PROXY="$TAG-proxy"
K=/opt/kafka/bin
WORK="$(mktemp -d)"
free_port() { python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])'; }

cleanup() {
  docker rm -f "$TAG-ro" "$KAFKA" "$PROXY" >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
  rm -rf "$WORK"
}
trap cleanup EXIT
dump() { for c in "$TAG-ro" "$KAFKA" "$PROXY"; do echo "--- $c"; docker logs "$c" 2>&1 | tail -30 || true; done; }

ver_out="$(docker run --rm "$IMAGE" --version 2>&1)"
echo "$ver_out"
grep -q '^kroxylicious: ' <<<"$ver_out" || { echo "no version line"; exit 1; }
if [ -n "${2:-}" ]; then
  grep -qx "kroxylicious: ${2}" <<<"$ver_out" || { echo "expected version $2"; exit 1; }
fi

# 2. read-only boot with the image's default config (its target broker does not exist,
#    which the proxy only notices when a client connects)
PORT="$(free_port)"
docker run -d --name "$TAG-ro" --read-only --tmpfs /tmp:exec -p "127.0.0.1:${PORT}:9190" "$IMAGE" >/dev/null
for i in $(seq 1 60); do
  code="$(curl -sS -m 3 -o /dev/null -w '%{http_code}' "http://127.0.0.1:${PORT}/livez" 2>/dev/null || true)"
  [ "$code" = 200 ] && break
  [ "$i" = 60 ] && { echo "/livez never answered on a read-only rootfs"; dump; exit 1; }
  sleep 1
done
curl -fsS -m 5 "http://127.0.0.1:${PORT}/metrics" | grep -q '^kroxylicious_build_info' \
  || { echo "no kroxylicious metrics"; exit 1; }
if docker logs "$TAG-ro" 2>&1 | grep -qiE 'read-only file system|permission denied|UnsatisfiedLinkError'; then
  echo "errors on a read-only rootfs:"; docker logs "$TAG-ro"; exit 1
fi
echo "read-only boot ok (/livez 200, metrics served)"

# 3. Kafka through the proxy
docker network create "$NET" >/dev/null
docker run -d --name "$KAFKA" --network "$NET" \
  -e KAFKA_EXTRA_PROPERTIES="advertised.listeners=PLAINTEXT://${KAFKA}:9092" "$KAFKA_IMAGE" >/dev/null
for i in $(seq 1 60); do
  docker exec "$KAFKA" "$K/kafka-broker-api-versions.sh" --bootstrap-server localhost:9092 >/dev/null 2>&1 && break
  [ "$i" = 60 ] && { echo "kafka did not become ready"; dump; exit 1; }
  sleep 2
done

cat > "$WORK/config.yaml" <<YAML
management:
  endpoints:
    prometheus: {}
clusterDefinitions:
  - name: backend
    bootstrapServers: ${KAFKA}:9092
virtualClusters:
  - name: smoke
    target:
      cluster: backend
    gateways:
      - name: default
        portIdentifiesNode:
          bootstrapAddress: 0.0.0.0:9192
          advertisedBrokerAddressPattern: ${PROXY}
YAML
docker create --name "$PROXY" --network "$NET" "$IMAGE" >/dev/null
docker cp "$WORK/config.yaml" "$PROXY:/etc/kroxylicious/config.yaml"
docker start "$PROXY" >/dev/null
BS="${PROXY}:9192"
for i in $(seq 1 60); do
  docker exec "$KAFKA" "$K/kafka-broker-api-versions.sh" --bootstrap-server "$BS" >/dev/null 2>&1 && break
  [ "$i" = 60 ] && { echo "the proxy never answered Kafka requests"; dump; exit 1; }
  sleep 2
done
docker exec "$KAFKA" "$K/kafka-topics.sh" --bootstrap-server "$BS" --create --topic smoke --partitions 1 --replication-factor 1
docker exec "$KAFKA" "$K/kafka-topics.sh" --bootstrap-server "$BS" --list | grep -qx smoke
echo "hello-through-kroxylicious-$$" | docker exec -i "$KAFKA" "$K/kafka-console-producer.sh" --bootstrap-server "$BS" --topic smoke
docker exec "$KAFKA" "$K/kafka-console-consumer.sh" --bootstrap-server "$BS" --topic smoke \
  --from-beginning --max-messages 1 --timeout-ms 30000 | grep -qx "hello-through-kroxylicious-$$" \
  || { echo "the message did not come back through the proxy"; dump; exit 1; }
echo "kafka round trip through the proxy ok"

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed (nonroot $user, read-only boot, Kafka produce/consume through the proxy)"
