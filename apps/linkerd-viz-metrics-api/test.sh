#!/usr/bin/env bash
# Smoke test for a built linkerd-viz-metrics-api image. Usage: test.sh <image-ref> [version]
# metrics-api starts its admin server (:9995), then k8s.InitializeAPI checks access before
# anything else: a SelfSubjectAccessReview and discovery of linkerd.io/v1alpha2 ServiceProfiles
# (controller/k8s/api.go initAPI, pkg/k8s/authz.go); a refused connection is fatal. A minimal fake
# API server (qw fakeapi on the runner, TLS) answers exactly those two calls and 404s the rest, so
# the real process gets past them and blocks in k8sAPI.Sync with the admin server up, like a pod
# before its informers sync. Serving stats needs a cluster and Prometheus: the chart's kind gate.
set -euo pipefail
# qw (tools/qw, Go) provides the helpers this test needs; go run reuses the build cache.
QWD="$(cd "$(dirname "$0")/../../tools/qw" && pwd)"; qw() { go -C "$QWD" run . "$@"; }
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
NAME="quench-viz-metrics-api-smoke-$$"
D="$(mktemp -d)"
FAKE=""
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; [ -z "$FAKE" ] || kill "$FAKE" 2>/dev/null || true; rm -rf "$D"; }
trap cleanup EXIT
# the fake API: SelfSubjectAccessReview allowed, and the linkerd.io/v1alpha2 resource list
printf '%s' '{"apiVersion":"authorization.k8s.io/v1","kind":"SelfSubjectAccessReview","status":{"allowed":true}}' > "$D/ssar.json"
printf '%s' '{"kind":"APIResourceList","apiVersion":"v1","groupVersion":"linkerd.io/v1alpha2","resources":[{"name":"serviceprofiles","namespaced":true,"kind":"ServiceProfile","verbs":["list","watch"]}]}' > "$D/res.json"
go -C "$QWD" build -o "$D/qw" .
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 1 -subj /CN=127.0.0.1 \
  -keyout "$D/api.key" -out "$D/api.crt" 2>/dev/null
"$D/qw" fakeapi --addr 127.0.0.1:16443 --cert "$D/api.crt" --key "$D/api.key" \
  --route "POST /apis/authorization.k8s.io/v1/selfsubjectaccessreviews=201:$D/ssar.json" \
  --route "GET /apis/linkerd.io/v1alpha2=200:$D/res.json" & FAKE=$!
cat > "$D/kubeconfig" <<'K'
apiVersion: v1
kind: Config
clusters:
- cluster: {server: "https://127.0.0.1:16443", insecure-skip-tls-verify: true}
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
docker run -d --name "$NAME" --network host -v "$D:/cfg:ro" "$IMAGE" -kubeconfig=/cfg/kubeconfig >/dev/null
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
