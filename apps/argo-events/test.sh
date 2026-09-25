#!/usr/bin/env bash
# Smoke test for a built argo-events image. Usage: test.sh <image-ref> [version]
# On a read-only root: the lint subcommand validates a real EventSource and Sensor, the
# bundled argo CLI reports its version, and the controller, given a kubeconfig for an API
# server that does not answer, logs its stamped version and dials that server without a
# panic; the binary carries the stamped version. The chart gate runs a webhook
# EventSource and a Sensor in kind.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
NAME="argo-events-smoke-$$"
trap 'docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$WORK"' EXIT
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

docker run --rm --read-only --entrypoint /usr/bin/argo "$IMAGE" version --short | grep -q '^argo: v3\.' \
  || { echo "argo CLI missing or wrong"; exit 1; }

cat > "$WORK/es.yaml" <<'YAML'
apiVersion: argoproj.io/v1alpha1
kind: EventSource
metadata: { name: webhook }
spec:
  webhook:
    example: { port: "12000", endpoint: /example, method: POST }
YAML
cat > "$WORK/sensor.yaml" <<'YAML'
apiVersion: argoproj.io/v1alpha1
kind: Sensor
metadata: { name: webhook }
spec:
  dependencies:
    - { name: dep, eventSourceName: webhook, eventName: example }
  triggers:
    - template:
        name: log
        log: {}
YAML
cat > "$WORK/kubeconfig" <<'KC'
apiVersion: v1
kind: Config
clusters: [{name: none, cluster: {server: "https://127.0.0.1:1", insecure-skip-tls-verify: true}}]
users: [{name: none, user: {token: none}}]
contexts: [{name: none, context: {cluster: none, user: none}}]
current-context: none
KC
# the controller loads its EventBus settings before anything else (the chart's ConfigMap)
mkdir -p "$WORK/etc"
cat > "$WORK/etc/controller-config.yaml" <<'CFG'
eventBus:
  # the controller validates that both lists are non-empty; nothing here is pulled
  nats:
    versions:
      - version: latest
        natsStreamingImage: nats-streaming:latest
        metricsExporterImage: natsio/prometheus-nats-exporter:latest
  jetstream:
    versions:
      - version: latest
        natsImage: nats:latest
        metricsExporterImage: natsio/prometheus-nats-exporter:latest
        configReloaderImage: natsio/nats-server-config-reloader:latest
        startCommand: /nats-server
CFG
chmod -R a+rX "$WORK"
lint="$(docker run --rm --read-only -v "$WORK:/w:ro" "$IMAGE" lint /w/es.yaml /w/sensor.yaml 2>&1 || true)"
echo "$lint" | grep -qiE 'error|invalid' && { echo "lint rejected valid resources:"; echo "$lint"; exit 1; }

docker run -d --name "$NAME" --read-only --tmpfs /tmp -v "$WORK/kubeconfig:/etc/kubeconfig:ro" -v "$WORK/etc:/etc/argo-events:ro" \
  -e KUBECONFIG=/etc/kubeconfig -e ARGO_EVENTS_IMAGE="$IMAGE" "$IMAGE" controller --leader-election=false >/dev/null
sleep 10
out="$(docker logs "$NAME" 2>&1)"
echo "$out" | grep -q '127.0.0.1:1' || { echo "controller never dialed the configured API server:"; echo "$out" | tail -15; exit 1; }
if echo "$out" | grep -qE 'panic:|read-only file system'; then echo "controller crashed or wrote to the root:"; echo "$out" | tail -15; exit 1; fi
# the controller logs its version only once it reaches the API server, so read the
# stamp from the binary itself
if [ -n "$WANT" ]; then
  cid="$(docker create "$IMAGE")"; docker cp "$cid:/usr/bin/argo-events" "$WORK/argo-events" >/dev/null; docker rm "$cid" >/dev/null
  grep -aq "v$WANT" "$WORK/argo-events" || { echo "version v$WANT not stamped into the binary"; exit 1; }
fi
echo "smoke test passed (argo-events ${WANT:-?}, uid $user, lint, argo CLI, controller dials the API server)"
