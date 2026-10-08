#!/usr/bin/env bash
# Smoke test for the Strimzi operator image. Usage: test.sh <image-ref> [version]
# 1) The cluster operator, launched exactly as upstream's Deployment does (args
#    only, no entrypoint), boots Vert.x + the fabric8 client and serves /healthy
#    on :8080. The API server is a TEST-NET blackhole (192.0.2.1), so the
#    operator sits in its connect timeout instead of exiting, which leaves the
#    health server up long enough to probe.
# 2) The topic operator, user operator and kafka-init scripts each reach their
#    Main (proves the filtered classpath in every *_run.sh resolves).
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
VER="${2:-}"
NAME="quench-strimzi-op-smoke-$$"

cleanup() { docker rm -f "$NAME" "$NAME-to" "$NAME-uo" "$NAME-init" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
ep="$(docker inspect "$IMAGE" --format '{{json .Config.Entrypoint}}')"
case "$ep" in null|'[]') ;; *) echo "expected no entrypoint, got $ep"; exit 1 ;; esac

# The image env the upstream Deployment (060-Deployment-strimzi-cluster-operator.yaml)
# sets; the operator refuses to start without its default image maps.
K=q.example/kafka:x
KMAP="$(printf '4.2.0=%s\n4.2.1=%s\n4.3.0=%s\n4.3.1=%s\n' $K $K $K $K)"
echo "starting the cluster operator"
docker run -d --name "$NAME" -p 127.0.0.1::8080 \
  -e STRIMZI_NAMESPACE=default -e STRIMZI_OPERATOR_NAMESPACE=default \
  -e STRIMZI_KAFKA_IMAGES="$KMAP" -e STRIMZI_KAFKA_CONNECT_IMAGES="$KMAP" \
  -e STRIMZI_KAFKA_MIRROR_MAKER_2_IMAGES="$KMAP" \
  -e STRIMZI_DEFAULT_KAFKA_EXPORTER_IMAGE=$K -e STRIMZI_DEFAULT_CRUISE_CONTROL_IMAGE=$K \
  -e STRIMZI_DEFAULT_TOPIC_OPERATOR_IMAGE=$K -e STRIMZI_DEFAULT_USER_OPERATOR_IMAGE=$K \
  -e STRIMZI_DEFAULT_KAFKA_INIT_IMAGE=$K -e STRIMZI_DEFAULT_KAFKA_BRIDGE_IMAGE=$K \
  -e STRIMZI_DEFAULT_KANIKO_EXECUTOR_IMAGE=$K -e STRIMZI_DEFAULT_BUILDAH_IMAGE=$K \
  -e STRIMZI_DEFAULT_MAVEN_BUILDER=$K \
  -e KUBERNETES_SERVICE_HOST=192.0.2.1 -e KUBERNETES_SERVICE_PORT=443 \
  "$IMAGE" /opt/strimzi/bin/cluster_operator_run.sh >/dev/null
port="$(docker port "$NAME" 8080/tcp | head -1 | sed 's/.*://')"
ok=0
for i in $(seq 1 60); do
  code="$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:${port}/healthy" || true)"
  if [ "$code" = 204 ]; then ok=1; echo "/healthy 204 after ~${i}s"; break; fi
  sleep 1
done
logs="$(docker logs "$NAME" 2>&1 || true)"
[ "$ok" = 1 ] || { echo "cluster operator never served /healthy"; tail -60 <<<"$logs"; exit 1; }
grep -q "ClusterOperator ${VER}.* is starting" <<<"$logs" || { echo "no ClusterOperator start line"; tail -60 <<<"$logs"; exit 1; }
grep -q 'Health and metrics server is ready on port 8080' <<<"$logs"
metrics="$(curl -s "http://127.0.0.1:${port}/metrics")"
grep -q '^jvm_' <<<"$metrics" || { echo "/metrics has no jvm_ series"; exit 1; }

# The other components: each must reach its Main and log its start line.
check_main() { # container-suffix script expected-log
  docker run -d --name "$NAME-$1" \
    -e STRIMZI_NAMESPACE=default -e STRIMZI_KAFKA_BOOTSTRAP_SERVERS=192.0.2.1:9092 \
    -e STRIMZI_CA_CERT_NAME=x -e STRIMZI_CA_KEY_NAME=x -e STRIMZI_CA_NAMESPACE=default \
    -e NODE_NAME=n -e KUBERNETES_SERVICE_HOST=192.0.2.1 -e KUBERNETES_SERVICE_PORT=443 \
    "$IMAGE" "/opt/strimzi/bin/$2" >/dev/null
  for i in $(seq 1 30); do
    out="$(docker logs "$NAME-$1" 2>&1 || true)"
    if grep -q "$3" <<<"$out"; then echo "$2 reached Main"; return 0; fi
    sleep 1
  done
  echo "$2 did not log '$3'"; tail -40 <<<"$out"; return 1
}
check_main to topic_operator_run.sh "TopicOperator ${VER}.* is starting"
check_main uo user_operator_run.sh "UserOperator ${VER}.* is starting"
check_main init kafka_init_run.sh "Init-kafka ${VER}.* is starting"

echo "smoke test passed (nonroot user: $user)"
