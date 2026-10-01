#!/usr/bin/env bash
# Smoke test for a built kube-vip image. Usage: test.sh <image-ref> [expected-version]
#
# Runs the manager in Services ARP mode against a kubeconfig that points at 127.0.0.1:1,
# where nothing answers: it must keep running, retry its leader lease, and serve the
# Prometheus endpoint. The chart's kind gate covers handing out a LoadBalancer IP.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
WANT="${2:-}"
NAME="quench-kube-vip-smoke-$$"
PORT=12112
KC="$(mktemp)"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -f "$KC"; }
trap cleanup EXIT
fail() { echo "FAIL: $*"; docker logs "$NAME" 2>&1 | tail -20; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
ver="$(docker run --rm "$IMAGE" version)"
echo "$ver"
[ -z "$WANT" ] || grep -q "^Version:  v$WANT$" <<<"$ver" || { echo "expected v$WANT"; exit 1; }

cat > "$KC" <<'KUBE'
apiVersion: v1
kind: Config
clusters: [{name: dead, cluster: {server: "https://127.0.0.1:1", insecure-skip-tls-verify: true}}]
users: [{name: dead, user: {token: smoke}}]
contexts: [{name: dead, context: {cluster: dead, user: dead}}]
current-context: dead
KUBE
chmod 0644 "$KC"

docker create --name "$NAME" -p "127.0.0.1:$PORT:2112" "$IMAGE" manager --services --arp \
  --k8sConfigPath=/kubeconfig --interface=eth0 --prometheusHTTPServer=:2112 >/dev/null
docker cp "$KC" "$NAME:/kubeconfig"
# 1.0 and 1.1 ignore --k8sConfigPath and read $HOME/.kube/config.
KD="$(mktemp -d)"; mkdir "$KD/.kube"; cp "$KC" "$KD/.kube/config"; chmod -R a+rX "$KD"
docker cp "$KD/." "$NAME:/home/kube-vip/"; rm -rf "$KD"
docker start "$NAME" >/dev/null
for i in $(seq 1 30); do
  body="$(curl -fsS --max-time 2 "http://127.0.0.1:$PORT/metrics" 2>/dev/null || true)"
  grep -q '^go_goroutines' <<<"$body" && break
  [ "$(docker inspect -f '{{.State.Status}}' "$NAME")" = running ] || fail "kube-vip exited"
  sleep 1
done
grep -q '^go_goroutines' <<<"$body" || fail "/metrics never answered"
for i in $(seq 1 20); do
  grep -q 'Error retrieving lease lock' <<<"$(docker logs "$NAME" 2>&1)" && break
  sleep 1
done
grep -q 'Error retrieving lease lock' <<<"$(docker logs "$NAME" 2>&1)" || fail "never tried the API server for its lease"
[ "$(docker inspect -f '{{.State.Status}}' "$NAME")" = running ] || fail "kube-vip is not running any more"
echo "smoke test passed (uid $user, retrying a dead API server, /metrics up)"
