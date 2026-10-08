#!/usr/bin/env bash
# Smoke test for the Strimzi Kafka image. Usage: test.sh <image-ref> [version]
# Runs /opt/kafka/kafka_run.sh exactly as the Strimzi cluster operator does (args
# only, custom-config volume, serviceaccount namespace file), with:
#   - a fake Kubernetes API (python on the runner) serving the two Secrets the
#     kafka-agent javaagent loads at startup (cluster CA cert, node cert + key),
#   - the JMX exporter javaagent switched on.
# Passes when Strimzi's own readiness probe (kafka_readiness.sh -> kafka-agent
# /v1/ready/, 204 only at broker state RUNNING) succeeds, a message round-trips
# through a topic in KRaft combined mode, and :9404 serves kafka_server metrics.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
NAME="quench-strimzi-kafka-smoke-$$"
HOST=smoke-kafka-0          # kafka_run.sh takes the broker id from the hostname suffix
W="$(mktemp -d)"
API_PID=""

cleanup() {
  docker rm -f "$NAME" >/dev/null 2>&1 || true
  [ -n "$API_PID" ] && kill "$API_PID" 2>/dev/null || true
  rm -rf "$W"
}
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
ep="$(docker inspect "$IMAGE" --format '{{json .Config.Entrypoint}}')"
case "$ep" in null|'[]') ;; *) echo "expected no entrypoint, got $ep"; exit 1 ;; esac

# --- certificates for the kafka-agent (PKCS8 RSA key, as Strimzi's CA issues) ---
openssl req -x509 -newkey rsa:2048 -nodes -keyout "$W/ca.key" -out "$W/ca.crt" -days 2 -subj "/CN=smoke-ca" 2>/dev/null
openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out "$W/node.key" 2>/dev/null
openssl req -new -key "$W/node.key" -out "$W/node.csr" -subj "/CN=$HOST" 2>/dev/null
openssl x509 -req -in "$W/node.csr" -CA "$W/ca.crt" -CAkey "$W/ca.key" -CAcreateserial -out "$W/node.crt" -days 2 2>/dev/null

# --- fake Kubernetes API: GET .../namespaces/default/secrets/<name> ---
cat > "$W/api.py" <<'PY'
import base64, http.server, json, sys
w, host = sys.argv[1], sys.argv[2]
b = lambda p: base64.b64encode(open(f"{w}/{p}", "rb").read()).decode()
secrets = {
    "smoke-cluster-ca-cert": {"ca.crt": b("ca.crt")},
    host: {f"{host}.crt": b("node.crt"), f"{host}.key": b("node.key")},
}
class H(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        name = self.path.split("?")[0].rstrip("/").rsplit("/", 1)[-1]
        if "/namespaces/default/secrets/" in self.path and name in secrets:
            body = {"apiVersion": "v1", "kind": "Secret", "type": "Opaque",
                    "metadata": {"name": name, "namespace": "default"}, "data": secrets[name]}
            code = 200
        else:
            body = {"kind": "Status", "apiVersion": "v1", "status": "Failure", "code": 404}
            code = 404
        out = json.dumps(body).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(out)))
        self.end_headers()
        self.wfile.write(out)
        sys.stderr.write(f"fake-api {code} {self.path}\n")
    def log_message(self, *a): pass
s = http.server.ThreadingHTTPServer(("0.0.0.0", 0), H)
open(f"{w}/port", "w").write(str(s.server_address[1]))
s.serve_forever()
PY
python3 "$W/api.py" "$W" "$HOST" 2>"$W/api.log" &
API_PID=$!
for _ in $(seq 1 50); do [ -s "$W/port" ] && break; sleep 0.1; done
PORT="$(cat "$W/port")"

# --- what the operator would mount ---
mkdir -p "$W/cfg" "$W/sa"
echo default > "$W/sa/namespace"
cat > "$W/cfg/server.config" <<CFG
node.id=0
process.roles=broker,controller
controller.listener.names=CONTROLLER
controller.quorum.voters=0@localhost:9090
listeners=CONTROLLER://:9090,PLAINTEXT://:9092
advertised.listeners=PLAINTEXT://localhost:9092
listener.security.protocol.map=CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT
inter.broker.listener.name=PLAINTEXT
log.dirs=/var/lib/kafka/data-0/kafka-log0
metadata.log.dir=/var/lib/kafka/data-0/kafka-log0
offsets.topic.replication.factor=1
transaction.state.log.replication.factor=1
transaction.state.log.min.isr=1
group.initial.rebalance.delay.ms=0
CFG
python3 -c 'import base64,uuid;print(base64.urlsafe_b64encode(uuid.uuid4().bytes).decode().rstrip("="))' > "$W/cfg/cluster.id"
echo "4.3-IV0" > "$W/cfg/metadata.version"
printf '%s\n' 'name=smoke' 'appender.console.type=Console' 'appender.console.name=STDOUT' \
  'appender.console.layout.type=PatternLayout' 'appender.console.layout.pattern=%d %p %c - %m%n' \
  'rootLogger.level=INFO' 'rootLogger.appenderRefs=console' 'rootLogger.appenderRef.console.ref=STDOUT' \
  > "$W/cfg/log4j2.properties"
echo '{"lowercaseOutputName": true, "rules": [{"pattern": ".*"}]}' > "$W/cfg/metrics-config.json"
chmod -R a+rX "$W"

echo "starting $IMAGE via /opt/kafka/kafka_run.sh"
docker run -d --name "$NAME" --hostname "$HOST" \
  --add-host host.docker.internal:host-gateway \
  -e KUBERNETES_MASTER="http://host.docker.internal:${PORT}" \
  -e KUBERNETES_AUTH_TRYKUBECONFIG=false -e KUBERNETES_AUTH_TRYSERVICEACCOUNT=false \
  -e KAFKA_CLUSTER_NAME=smoke -e KAFKA_HEAP_OPTS="-Xms512m -Xmx512m" \
  -e KAFKA_JMX_EXPORTER_ENABLED=true \
  -v "$W/cfg:/opt/kafka/custom-config:ro" \
  -v "$W/sa:/var/run/secrets/kubernetes.io/serviceaccount:ro" \
  --tmpfs /var/lib/kafka:uid=1001,gid=1001,mode=0755 \
  "$IMAGE" /opt/kafka/kafka_run.sh >/dev/null

ready=0
for i in $(seq 1 90); do
  if docker exec "$NAME" /opt/kafka/kafka_readiness.sh >/dev/null 2>&1; then ready=1; echo "kafka_readiness.sh passed after ~$((i*2))s"; break; fi
  [ "$(docker inspect -f '{{.State.Running}}' "$NAME")" = true ] || break
  sleep 2
done
if [ "$ready" != 1 ]; then
  echo "broker never became ready"; docker logs "$NAME" 2>&1 | tail -80; echo "--- fake api"; cat "$W/api.log"; exit 1
fi

B=localhost:9092
x() { docker exec -e LOG_DIR=/tmp -e KAFKA_OPTS= "$NAME" "$@"; }
x /opt/kafka/bin/kafka-topics.sh --bootstrap-server "$B" --create --topic smoke --partitions 1 --replication-factor 1
echo "hello-quenchworks" | docker exec -i -e LOG_DIR=/tmp "$NAME" /opt/kafka/bin/kafka-console-producer.sh --bootstrap-server "$B" --topic smoke
out="$(x /opt/kafka/bin/kafka-console-consumer.sh --bootstrap-server "$B" --topic smoke --from-beginning --max-messages 1 --timeout-ms 20000)"
grep -q 'hello-quenchworks' <<<"$out" || { echo "message did not round-trip"; exit 1; }
echo "message round-tripped"

m="$(x curl -s http://localhost:9404/metrics)"
grep -q '^kafka_server_' <<<"$m" || { echo "JMX exporter served no kafka_server_ metrics"; exit 1; }
echo "JMX exporter serves kafka_server_ metrics"

kv="$(x /opt/kafka-exporter/kafka_exporter --version 2>&1 || true)"
grep -q 'version' <<<"$kv" || { echo "kafka_exporter --version failed: $kv"; exit 1; }
x sh -c 'ls /opt/cruise-control/libs/cruise-control-[0-9]*.jar >/dev/null && test -x /opt/cruise-control/cruise_control_run.sh'

echo "smoke test passed (nonroot user: $user)"
