#!/usr/bin/env bash
# Smoke test for a built datahub-upgrade image. Usage: test.sh <image-ref> [version]
# Real setup work: the QuenchWorks MySQL, Kafka and Elasticsearch images come up on one
# network and the image runs `-u SystemUpdate` on a read-only root, as a fresh install
# does. It must report SUCCEEDED and leave the SQL table, the Kafka topics and the
# search indices behind.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
MYSQL=ghcr.io/quenchworks/images/mysql@sha256:40b6296bb2cea66b225f8d26bbecd4a11859dce69e48a7267fcfd38e58ac052e
KAFKA=ghcr.io/quenchworks/images/kafka@sha256:6b7ea2f5c99d48e4ad40f686464687791b50e55d49b6b74b04985f38573febd2
ES=ghcr.io/quenchworks/images/elasticsearch@sha256:f2b65a8c303e1591bcfde5a62435b0b97d60fc7a4a1399ced11ad8eb479ecda6
T="dhup-smoke-$$"; NET="$T-net"; PW="db-$$-secret"; LOG="$(mktemp)"
cleanup() { rm -f "$LOG"; docker rm -f "$T-mysql" "$T-kafka" "$T-es" "$T-up" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
[ -n "${KEEP:-}" ] || trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

docker network create "$NET" >/dev/null
docker run -d --name "$T-mysql" --network "$NET" --network-alias mysql -e MYSQL_ROOT_PASSWORD="$PW" -e MYSQL_DATABASE=datahub "$MYSQL" >/dev/null
docker run -d --name "$T-kafka" --network "$NET" --network-alias kafka -e KAFKA_ADVERTISED_LISTENERS=PLAINTEXT://kafka:9092 "$KAFKA" >/dev/null
docker run -d --name "$T-es" --network "$NET" --network-alias search -p 127.0.0.1:19201:9200 -e ES_JAVA_OPTS="-Xms512m -Xmx512m" "$ES" >/dev/null
for i in $(seq 1 90); do
  docker exec "$T-mysql" mysqladmin --socket=/run/mysqld/mysqld.sock -uroot -p"$PW" ping >/dev/null 2>&1 \
    && curl -fsS http://127.0.0.1:19201/ >/dev/null 2>&1 \
    && docker exec "$T-kafka" /opt/kafka/bin/kafka-broker-api-versions.sh --bootstrap-server localhost:9092 >/dev/null 2>&1 && break
  [ "$i" = 90 ] && { echo "dependencies did not become ready"; exit 1; }
  sleep 2
done

rc=0
# a failed Spring context can leave the JVM up; bound the run
timeout 480 docker run --name "$T-up" --network "$NET" --read-only --tmpfs /tmp:exec \
  -e EBEAN_DATASOURCE_USERNAME=root -e EBEAN_DATASOURCE_PASSWORD="$PW" -e EBEAN_DATASOURCE_HOST=mysql:3306 \
  -e 'EBEAN_DATASOURCE_URL=jdbc:mysql://mysql:3306/datahub?verifyServerCertificate=false&useSSL=true&useUnicode=yes&characterEncoding=UTF-8&enabledTLSProtocols=TLSv1.2' \
  -e EBEAN_DATASOURCE_DRIVER=com.mysql.jdbc.Driver \
  -e GRAPH_SERVICE_IMPL=elasticsearch -e ELASTICSEARCH_HOST=search -e ELASTICSEARCH_PORT=9200 -e ELASTICSEARCH_USE_SSL=false \
  -e KAFKA_BOOTSTRAP_SERVER=kafka:9092 -e SCHEMA_REGISTRY_TYPE=INTERNAL -e KAFKA_SCHEMAREGISTRY_URL=http://gms:8080/schema-registry/api/ \
  -e USE_CONFLUENT_SCHEMA_REGISTRY=false -e DATAHUB_PRECREATE_TOPICS=true -e DATAHUB_SQL_SETUP_ENABLED=true -e PARTITIONS=1 \
  -e DATAHUB_SYSTEM_CLIENT_SECRET="sys-$$-secret" -e DATAHUB_TOKEN_SERVICE_SIGNING_KEY="sign-$$-0123456789abcdef0123456789" \
  -e DATAHUB_TOKEN_SERVICE_SALT="salt-$$" -e JDK_JAVA_OPTIONS=-Xmx768m \
  "$IMAGE" -u SystemUpdate > "$LOG" 2>&1 || rc=$?
log="$(cat "$LOG")"
if [ "$rc" != 0 ] || ! grep -q 'Upgrade SystemUpdate completed with result SUCCEEDED' <<<"$log"; then
  echo "SystemUpdate failed (rc=$rc)"; grep -E 'ERROR|Exception|Caused by' <<<"$log" | head -30; tail -20 <<<"$log"; exit 1
fi

tables="$(docker exec "$T-mysql" mysql --socket=/run/mysqld/mysqld.sock -uroot -p"$PW" -N -e 'SHOW TABLES' datahub 2>/dev/null)"
grep -qx metadata_aspect_v2 <<<"$tables" || { echo "no metadata_aspect_v2 table: $tables"; exit 1; }
topics="$(docker exec "$T-kafka" /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --list)"
grep -qx MetadataChangeLog_Versioned_v1 <<<"$topics" || { echo "no MCL topic: $topics"; exit 1; }
indices="$(curl -fsS 'http://127.0.0.1:19201/_cat/indices?h=index')"
grep -qx datasetindex_v2 <<<"$indices" || { echo "no dataset index: $(head -c 400 <<<"$indices")"; exit 1; }
echo "smoke test passed (datahub-upgrade ${WANT:-?}, uid $user, SystemUpdate SUCCEEDED: table, $(grep -c . <<<"$topics") topics, $(grep -c . <<<"$indices") indices)"
