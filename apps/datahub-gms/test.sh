#!/usr/bin/env bash
# Smoke test for a built datahub-gms image. Usage: test.sh <image-ref> [version]
# Real metadata flow: the QuenchWorks MySQL, Kafka and Elasticsearch images come up, the
# QuenchWorks datahub-upgrade image runs SystemUpdate, then GMS starts on a read-only root
# and must report healthy and its version, accept a dataset through the REST.li
# ingestProposal API, return it through GraphQL, and find it through search (which only
# works once the embedded MAE consumer has carried the change from Kafka to the index).
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
UPGRADE="${DATAHUB_UPGRADE_IMAGE:-ghcr.io/quenchworks/images/datahub-upgrade@sha256:7d4452a171f27fc059ff306181ea22c8aa13fb057579e76842788d41095a1576}"
MYSQL=ghcr.io/quenchworks/images/mysql@sha256:40b6296bb2cea66b225f8d26bbecd4a11859dce69e48a7267fcfd38e58ac052e
KAFKA=ghcr.io/quenchworks/images/kafka@sha256:6b7ea2f5c99d48e4ad40f686464687791b50e55d49b6b74b04985f38573febd2
ES=ghcr.io/quenchworks/images/elasticsearch@sha256:f2b65a8c303e1591bcfde5a62435b0b97d60fc7a4a1399ced11ad8eb479ecda6
T="dhgms-smoke-$$"; NET="$T-net"; PW="db-$$-secret"; SYS="sys-$$-secret"; LOG="$(mktemp)"
GMS=http://127.0.0.1:18088
cleanup() { rm -f "$LOG"; docker rm -f "$T-mysql" "$T-kafka" "$T-es" "$T-up" "$T-gms" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
[ -n "${KEEP:-}" ] || trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

docker network create "$NET" >/dev/null
docker run -d --name "$T-mysql" --network "$NET" --network-alias mysql -e MYSQL_ROOT_PASSWORD="$PW" -e MYSQL_DATABASE=datahub "$MYSQL" >/dev/null
docker run -d --name "$T-kafka" --network "$NET" --network-alias kafka -e KAFKA_ADVERTISED_LISTENERS=PLAINTEXT://kafka:9092 "$KAFKA" >/dev/null
docker run -d --name "$T-es" --network "$NET" --network-alias search -p 127.0.0.1:19202:9200 -e ES_JAVA_OPTS="-Xms512m -Xmx512m" "$ES" >/dev/null
for i in $(seq 1 90); do
  docker exec "$T-mysql" mysqladmin --socket=/run/mysqld/mysqld.sock -uroot -p"$PW" ping >/dev/null 2>&1 \
    && docker exec "$T-kafka" /opt/kafka/bin/kafka-broker-api-versions.sh --bootstrap-server localhost:9092 >/dev/null 2>&1 \
    && curl -fsS http://127.0.0.1:19202/ >/dev/null 2>&1 && break
  [ "$i" = 90 ] && { echo "dependencies did not become ready"; exit 1; }
  sleep 2
done

DH=(-e EBEAN_DATASOURCE_USERNAME=root -e EBEAN_DATASOURCE_PASSWORD="$PW" -e EBEAN_DATASOURCE_HOST=mysql:3306
  -e 'EBEAN_DATASOURCE_URL=jdbc:mysql://mysql:3306/datahub?verifyServerCertificate=false&useSSL=true&useUnicode=yes&characterEncoding=UTF-8&enabledTLSProtocols=TLSv1.2'
  -e EBEAN_DATASOURCE_DRIVER=com.mysql.jdbc.Driver
  -e GRAPH_SERVICE_IMPL=elasticsearch -e ELASTICSEARCH_HOST=search -e ELASTICSEARCH_PORT=9200 -e ELASTICSEARCH_USE_SSL=false
  -e KAFKA_BOOTSTRAP_SERVER=kafka:9092 -e SCHEMA_REGISTRY_TYPE=INTERNAL -e KAFKA_SCHEMAREGISTRY_URL=http://gms:8080/schema-registry/api/
  -e DATAHUB_SYSTEM_CLIENT_SECRET="$SYS" -e DATAHUB_TOKEN_SERVICE_SIGNING_KEY="sign-$$-0123456789abcdef0123456789"
  -e DATAHUB_TOKEN_SERVICE_SALT="salt-$$" -e JDK_JAVA_OPTIONS=-Xmx1g)

rc=0
timeout 480 docker run --name "$T-up" --network "$NET" --read-only --tmpfs /tmp:exec "${DH[@]}" \
  -e USE_CONFLUENT_SCHEMA_REGISTRY=false -e DATAHUB_PRECREATE_TOPICS=true -e DATAHUB_SQL_SETUP_ENABLED=true -e PARTITIONS=1 \
  "$UPGRADE" -u SystemUpdate > "$LOG" 2>&1 || rc=$?
grep -q 'Upgrade SystemUpdate completed with result SUCCEEDED' "$LOG" || { echo "SystemUpdate failed (rc=$rc)"; grep -E 'ERROR|Caused by' "$LOG" | head -20; exit 1; }

# Kafka snappy and zstd natives unpack into /tmp and load from there: it must allow exec.
# The consumers are off unless enabled (upstream compose and helm turn them on).
docker run -d --name "$T-gms" --network "$NET" --network-alias gms --read-only --tmpfs /tmp:exec -p 127.0.0.1:18088:8080 \
  "${DH[@]}" -e METADATA_SERVICE_AUTH_ENABLED=true -e DATAHUB_TELEMETRY_ENABLED=false \
  -e MAE_CONSUMER_ENABLED=true -e MCE_CONSUMER_ENABLED=true -e PE_CONSUMER_ENABLED=true "$IMAGE" >/dev/null
ok=0
for _ in $(seq 1 120); do
  curl -fsS "$GMS/health" >/dev/null 2>&1 && { ok=1; break; }
  [ "$(docker inspect -f '{{.State.Running}}' "$T-gms")" = true ] || break
  sleep 3
done
[ "$ok" = 1 ] || { echo "GMS never became healthy"; docker logs "$T-gms" 2>&1 | grep -E 'ERROR|Caused by' | head -20; docker logs "$T-gms" 2>&1 | tail -20; exit 1; }

cfg="$(curl -fsS "$GMS/config")"
[ -z "$WANT" ] || grep -q "\"version\" : \"v$WANT\"" <<<"$cfg" || { echo "expected v$WANT in /config: $(head -c 400 <<<"$cfg")"; exit 1; }

A="Authorization: Basic __datahub_system:$SYS"
URN='urn:li:dataset:(urn:li:dataPlatform:hive,quench.smoke,PROD)'
code="$(curl -sS -o /dev/null -w '%{http_code}' "$GMS/api/graphql" -H 'Content-Type: application/json' -d '{"query":"{ me { corpUser { urn } } }"}')"
[ "$code" = 401 ] || { echo "anonymous GraphQL answered $code, expected 401"; exit 1; }
res="$(curl -fsS -X POST "$GMS/aspects?action=ingestProposal" -H "$A" -H 'Content-Type: application/json' -H 'X-RestLi-Protocol-Version: 2.0.0' \
  -d "{\"proposal\":{\"entityType\":\"dataset\",\"entityUrn\":\"$URN\",\"changeType\":\"UPSERT\",\"aspectName\":\"datasetProperties\",\"aspect\":{\"value\":\"{\\\"name\\\":\\\"quench-smoke\\\",\\\"description\\\":\\\"from the image smoke test\\\"}\",\"contentType\":\"application/json\"}}}")"
grep -q '"value":"success"' <<<"$res" || { echo "ingest failed: $res"; exit 1; }
got="$(curl -fsS "$GMS/api/graphql" -H "$A" -H 'Content-Type: application/json' -d "{\"query\":\"{ dataset(urn: \\\"$URN\\\") { properties { name description } } }\"}")"
grep -q '"name":"quench-smoke"' <<<"$got" || { echo "GraphQL did not return the dataset: $got"; exit 1; }
found=0
for _ in $(seq 1 30); do
  s="$(curl -fsS "$GMS/api/graphql" -H "$A" -H 'Content-Type: application/json' -d '{"query":"{ search(input:{type:DATASET, query:\"quench\", start:0, count:5}) { total } }"}')"
  grep -q '"total":1' <<<"$s" && { found=1; break; }
  sleep 2
done
[ "$found" = 1 ] || { echo "dataset never reached the search index: $s"; exit 1; }
echo "smoke test passed (datahub-gms ${WANT:-?}, uid $user, dataset ingested, read back over GraphQL, found by search)"
