#!/usr/bin/env bash
# Smoke test for a built datahub-frontend image. Usage: test.sh <image-ref> [version]
# The whole stack on QuenchWorks images: MySQL, Kafka and Elasticsearch, datahub-upgrade's
# SystemUpdate, GMS, then the frontend on a read-only root with a generated admin
# password. The upstream default login must be refused, the generated one accepted; the
# session mints an access token through the frontend GraphQL proxy, the token ingests a
# dataset into GMS, and the dataset reads back through the frontend.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
UPGRADE="${DATAHUB_UPGRADE_IMAGE:-ghcr.io/quenchworks/images/datahub-upgrade@__UPGRADE_DIGEST__}"
GMSIMG="${DATAHUB_GMS_IMAGE:-ghcr.io/quenchworks/images/datahub-gms@__GMS_DIGEST__}"
MYSQL=ghcr.io/quenchworks/images/mysql@sha256:40b6296bb2cea66b225f8d26bbecd4a11859dce69e48a7267fcfd38e58ac052e
KAFKA=ghcr.io/quenchworks/images/kafka@sha256:6b7ea2f5c99d48e4ad40f686464687791b50e55d49b6b74b04985f38573febd2
ES=ghcr.io/quenchworks/images/elasticsearch@sha256:f2b65a8c303e1591bcfde5a62435b0b97d60fc7a4a1399ced11ad8eb479ecda6
T="dhfe-smoke-$$"; NET="$T-net"; PW="db-$$-secret"; SYS="sys-$$-secret"; LOG="$(mktemp)"
GMS=http://127.0.0.1:18089; FE=http://127.0.0.1:19002; ADMIN="adm-$$-$RANDOM"
cleanup() { rm -rf "$LOG" "$WORK"; docker rm -f "$T-mysql" "$T-kafka" "$T-es" "$T-up" "$T-gms" "$T-fe" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
[ -n "${KEEP:-}" ] || trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

docker network create "$NET" >/dev/null
docker run -d --name "$T-mysql" --network "$NET" --network-alias mysql -e MYSQL_ROOT_PASSWORD="$PW" -e MYSQL_DATABASE=datahub "$MYSQL" >/dev/null
docker run -d --name "$T-kafka" --network "$NET" --network-alias kafka -e KAFKA_ADVERTISED_LISTENERS=PLAINTEXT://kafka:9092 "$KAFKA" >/dev/null
docker run -d --name "$T-es" --network "$NET" --network-alias search -p 127.0.0.1:19203:9200 -e ES_JAVA_OPTS="-Xms512m -Xmx512m" "$ES" >/dev/null
for i in $(seq 1 90); do
  docker exec "$T-mysql" mysqladmin --socket=/run/mysqld/mysqld.sock -uroot -p"$PW" ping >/dev/null 2>&1 \
    && docker exec "$T-kafka" /opt/kafka/bin/kafka-broker-api-versions.sh --bootstrap-server localhost:9092 >/dev/null 2>&1 \
    && curl -fsS http://127.0.0.1:19203/ >/dev/null 2>&1 && break
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
grep -q "Upgrade SystemUpdate completed with result SUCCEEDED" "$LOG" || { echo "SystemUpdate failed (rc=$rc)"; tail -5 "$LOG"; grep -E 'ERROR|Caused by' "$LOG" | head -20; exit 1; }

# Kafka snappy and zstd natives unpack into /tmp and load from there: it must allow exec.
# The consumers are off unless enabled (upstream compose and helm turn them on).
docker run -d --name "$T-gms" --network "$NET" --network-alias gms --read-only --tmpfs /tmp:exec -p 127.0.0.1:18089:8080 \
  "${DH[@]}" -e METADATA_SERVICE_AUTH_ENABLED=true -e DATAHUB_TELEMETRY_ENABLED=false \
  -e MAE_CONSUMER_ENABLED=true -e MCE_CONSUMER_ENABLED=true -e PE_CONSUMER_ENABLED=true "$GMSIMG" >/dev/null
ok=0
for _ in $(seq 1 120); do
  curl -fsS "$GMS/health" >/dev/null 2>&1 && { ok=1; break; }
  [ "$(docker inspect -f '{{.State.Running}}' "$T-gms")" = true ] || break
  sleep 3
done
[ "$ok" = 1 ] || { echo "GMS never became healthy"; docker logs "$T-gms" 2>&1 | grep -E 'ERROR|Caused by' | head -20; docker logs "$T-gms" 2>&1 | tail -20; exit 1; }

printf 'datahub:%s\n' "$ADMIN" > "$WORK/user.props"
chmod -R a+rX "$WORK"
docker run -d --name "$T-fe" --network "$NET" --read-only --tmpfs /tmp -p 127.0.0.1:19002:9002 \
  -v "$WORK/user.props:/datahub-frontend/conf/user.props:ro" \
  -e DATAHUB_GMS_HOST=gms -e DATAHUB_GMS_PORT=8080 -e DATAHUB_SYSTEM_CLIENT_SECRET="$SYS" \
  -e DATAHUB_SECRET="$(printf 'fe-%s-%064d' $$ 0 | head -c 64)" -e KAFKA_BOOTSTRAP_SERVER=kafka:9092 \
  -e ELASTIC_CLIENT_HOST=search -e ELASTIC_CLIENT_PORT=9200 -e JDK_JAVA_OPTIONS=-Xmx512m "$IMAGE" >/dev/null
ok=0
for _ in $(seq 1 90); do
  curl -fsS "$FE/admin" >/dev/null 2>&1 && { ok=1; break; }
  [ "$(docker inspect -f '{{.State.Running}}' "$T-fe")" = true ] || break
  sleep 2
done
[ "$ok" = 1 ] || { echo "frontend never answered"; docker logs "$T-fe" 2>&1 | tail -30; exit 1; }

login() { curl -sS -o /dev/null -w '%{http_code}' -c "$WORK/jar" "$FE/logIn" -H 'Content-Type: application/json' -d "{\"username\":\"datahub\",\"password\":\"$1\"}"; }
code="$(login datahub)"
[ "$code" != 200 ] || { echo "the upstream default password datahub:datahub was accepted"; exit 1; }
code="$(login "$ADMIN")"
[ "$code" = 200 ] || { echo "login with the generated password answered $code"; docker logs "$T-fe" 2>&1 | tail -20; exit 1; }
gql() { curl -fsS -b "$WORK/jar" "$FE/api/v2/graphql" -H 'Content-Type: application/json' -d "$1"; }
tok="$(gql '{"query":"mutation { createAccessToken(input:{type:PERSONAL, actorUrn:\"urn:li:corpuser:datahub\", duration:ONE_HOUR, name:\"smoke\"}) { accessToken } }"}' | sed -n 's/.*"accessToken":"\([^"]*\)".*/\1/p')"
[ -n "$tok" ] || { echo "no access token minted"; exit 1; }
URN='urn:li:dataset:(urn:li:dataPlatform:hive,quench.smoke,PROD)'
res="$(curl -fsS -X POST "$GMS/aspects?action=ingestProposal" -H "Authorization: Bearer $tok" -H 'Content-Type: application/json' -H 'X-RestLi-Protocol-Version: 2.0.0' \
  -d "{\"proposal\":{\"entityType\":\"dataset\",\"entityUrn\":\"$URN\",\"changeType\":\"UPSERT\",\"aspectName\":\"datasetProperties\",\"aspect\":{\"value\":\"{\\\"name\\\":\\\"quench-smoke\\\"}\",\"contentType\":\"application/json\"}}}")"
grep -q '"value":"success"' <<<"$res" || { echo "ingest with the token failed: $res"; exit 1; }
got="$(gql "{\"query\":\"{ dataset(urn: \\\"$URN\\\") { properties { name } } }\"}")"
grep -q '"name":"quench-smoke"' <<<"$got" || { echo "frontend GraphQL did not return the dataset: $got"; exit 1; }
if docker logs "$T-fe" 2>&1 | grep -E ' ERROR '; then echo "frontend logged errors"; exit 1; fi
echo "smoke test passed (datahub-frontend ${WANT:-?}, uid $user, generated login, token minted, dataset ingested and read back)"
