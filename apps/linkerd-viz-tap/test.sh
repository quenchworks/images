#!/usr/bin/env bash
# Smoke test for a built linkerd-viz-tap image. Usage: test.sh <image-ref> [version]
# Runs the tap-injector role (`injector`), as the viz chart does: controller/webhook/launcher.go
# starts the admin server (:9995), parses the kubeconfig (no dial), loads the webhook TLS pair from
# /var/run/linkerd/tls, starts the webhook server (:8443) and blocks in metadataAPI.Sync. With an
# unreachable kubeconfig and a throwaway cert the real process must stay up, answer /ping and serve
# TLS on :8443. The `api` role needs the cluster's extension-apiserver-authentication ConfigMap
# before it can serve: the chart's kind gate covers it.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
NAME="quench-viz-tap-smoke-$$"
D="$(mktemp -d)"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$D"; }
trap cleanup EXIT
mkdir -p "$D/tls"
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
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 1 -subj /CN=tap-injector \
  -keyout "$D/tls/tls.key" -out "$D/tls/tls.crt" 2>/dev/null
chmod -R a+rX "$D"; chmod 644 "$D/tls/tls.key"
docker run -d --name "$NAME" -p 127.0.0.1:9995:9995 -p 127.0.0.1:8443:8443 \
  -v "$D/tls:/var/run/linkerd/tls:ro" -v "$D/kubeconfig:/cfg/kubeconfig:ro" \
  "$IMAGE" injector -kubeconfig=/cfg/kubeconfig -tap-service-name=tap.linkerd-viz.serviceaccount.identity.linkerd.cluster.local >/dev/null
for i in $(seq 1 30); do
  pong="$(curl -sS --max-time 3 http://127.0.0.1:9995/ping 2>/dev/null || true)"
  [ "$pong" = pong ] && break
  [ "$i" = 30 ] && { echo "admin server (:9995/ping) did not answer"; docker logs "$NAME"; exit 1; }
  sleep 1
done
code=""
for i in $(seq 1 15); do
  code="$(curl -sk -o /dev/null -w '%{http_code}' --max-time 3 https://127.0.0.1:8443/ 2>/dev/null || true)"
  case "$code" in [1-5]??) break ;; esac
  sleep 1
done
echo "webhook :8443 -> HTTP $code"
case "$code" in [1-5]??) ;; *) echo "webhook TLS server did not answer"; docker logs "$NAME"; exit 1 ;; esac
docker inspect -f '{{.State.Running}}' "$NAME" | grep -q true || { echo "process exited"; docker logs "$NAME"; exit 1; }
docker logs "$NAME" 2>&1 | tail -3
echo "smoke test passed"
