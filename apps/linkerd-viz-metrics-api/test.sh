#!/usr/bin/env bash
# Smoke test for a built linkerd-viz-metrics-api image. Usage: test.sh <image-ref> [version]
# metrics-api starts its admin server (:9995) in a goroutine, then builds its k8s client (parse
# only, no dial) and blocks in k8sAPI.Sync against the API server (viz/metrics-api/cmd/main.go).
# With a valid but unreachable kubeconfig the real process must come up and answer /ping and
# /metrics. Serving stats needs a cluster and Prometheus: the chart's kind gate.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
NAME="quench-viz-metrics-api-smoke-$$"
D="$(mktemp -d)"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$D"; }
trap cleanup EXIT
cat > "$D/kubeconfig" <<'K'
apiVersion: v1
kind: Config
clusters:
- cluster: {server: "https://127.0.0.1:6443", insecure-skip-tls-verify: true}
  name: standalone
contexts:
- context: {cluster: standalone, user: standalone}
  name: standalone
current-context: standalone
users:
- name: standalone
  user: {token: standalone-test-token}
K
chmod 755 "$D"; chmod 644 "$D/kubeconfig"
docker run -d --name "$NAME" -p 127.0.0.1:9995:9995 -v "$D:/cfg:ro" "$IMAGE" -kubeconfig=/cfg/kubeconfig >/dev/null
for i in $(seq 1 30); do
  pong="$(curl -sS --max-time 3 http://127.0.0.1:9995/ping 2>/dev/null || true)"
  [ "$pong" = pong ] && break
  [ "$i" = 30 ] && { echo "admin server (:9995/ping) did not answer"; docker logs "$NAME"; exit 1; }
  sleep 1
done
m="$(curl -sS --max-time 3 http://127.0.0.1:9995/metrics)"
grep -q '^# TYPE go_goroutines' <<<"$m" || { echo "no Prometheus exposition on /metrics"; exit 1; }
docker inspect -f '{{.State.Running}}' "$NAME" | grep -q true || { echo "process exited"; docker logs "$NAME"; exit 1; }
docker logs "$NAME" 2>&1 | tail -3
echo "smoke test passed"
