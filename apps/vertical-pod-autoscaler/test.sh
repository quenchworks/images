#!/usr/bin/env bash
# Smoke test for a built vertical-pod-autoscaler image. Usage: test.sh <image-ref> [version]
#
# Each component runs against a kubeconfig pointing at 127.0.0.1:1, where nothing
# answers: it must keep running, retry the API, and serve its metrics port. The
# admission controller also gets a throwaway CA and serving cert; its webhook port only
# opens after the informer cache syncs (pkg/admission-controller/main.go), so the
# chart's kind gate covers the webhook and real recommendations.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
TMP="$(mktemp -d)"
NAMES=()
cleanup() { for n in "${NAMES[@]}"; do docker rm -f "$n" >/dev/null 2>&1 || true; done; rm -rf "$TMP"; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

cat > "$TMP/kc" <<'KUBE'
apiVersion: v1
kind: Config
clusters: [{name: dead, cluster: {server: "https://127.0.0.1:1", insecure-skip-tls-verify: true}}]
users: [{name: dead, user: {token: smoke}}]
contexts: [{name: dead, context: {cluster: dead, user: dead}}]
current-context: dead
KUBE
openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj /CN=vpa-ca -keyout "$TMP/caKey.pem" -out "$TMP/caCert.pem" 2>/dev/null
openssl req -newkey rsa:2048 -nodes -subj /CN=vpa-webhook -keyout "$TMP/serverKey.pem" -out "$TMP/server.csr" 2>/dev/null
openssl x509 -req -in "$TMP/server.csr" -CA "$TMP/caCert.pem" -CAkey "$TMP/caKey.pem" -CAcreateserial -days 1 -out "$TMP/serverCert.pem" 2>/dev/null
chmod -R a+rX "$TMP"

# run <component> <host-metrics-port> <container-metrics-port> [extra args...]
run() {
  local c="$1" hp="$2" cp="$3"; shift 3
  local n="quench-vpa-$c-$$"; NAMES+=("$n")
  docker create --name "$n" -p "127.0.0.1:$hp:$cp" "${PUBLISH[@]}" --entrypoint "/usr/bin/vpa-$c" "$IMAGE" --kubeconfig=/work/kc "$@" >/dev/null
  docker cp "$TMP/." "$n:/work"
  docker start "$n" >/dev/null
  for i in $(seq 1 30); do
    [ "$(curl -s -o /dev/null -w '%{http_code}' --max-time 2 "http://127.0.0.1:$hp/metrics" || true)" = 200 ] && break
    [ "$(docker inspect -f '{{.State.Status}}' "$n")" = running ] || { docker logs "$n" 2>&1 | tail -20; echo "FAIL: $c exited"; exit 1; }
    sleep 1
  done
  [ "$(curl -s -o /dev/null -w '%{http_code}' --max-time 2 "http://127.0.0.1:$hp/metrics")" = 200 ] || { docker logs "$n" 2>&1 | tail -20; echo "FAIL: $c /metrics"; exit 1; }
  echo "$c: running, /metrics 200"
}

PUBLISH=(); run recommender 18942 8942
PUBLISH=(); run updater 18943 8943
PUBLISH=()
run admission-controller 18944 8944 --register-webhook=false \
  --client-ca-file=/work/caCert.pem --tls-cert-file=/work/serverCert.pem --tls-private-key=/work/serverKey.pem
echo "smoke test passed (uid $user, three components up against a dead API server)"
