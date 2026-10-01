#!/usr/bin/env bash
# Smoke test for a built Reloader image. Usage: test.sh <image-ref> [expected-version]
#
# Reloader is a Kubernetes controller; the chart's kind gate proves it rolls a workload.
# Here it runs against a kubeconfig that points at 127.0.0.1:1, which nothing answers: the
# client builds, the controllers start and retry, and the HTTP server comes up. The image
# has no shell, so the kubeconfig goes in with docker cp.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
NAME="quench-reloader-smoke-$$"
PORT=19090
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -f "$KC"; }
KC="$(mktemp)"
trap cleanup EXIT
fail() { echo "FAIL: $*"; docker logs "$NAME" 2>&1 | tail -20; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

cat > "$KC" <<'KUBE'
apiVersion: v1
kind: Config
clusters: [{name: dead, cluster: {server: "https://127.0.0.1:1", insecure-skip-tls-verify: true}}]
users: [{name: dead, user: {token: smoke}}]
contexts: [{name: dead, context: {cluster: dead, user: dead}}]
current-context: dead
KUBE
chmod 0644 "$KC"

docker create --name "$NAME" -p "127.0.0.1:$PORT:9090" -e KUBECONFIG=/kubeconfig "$IMAGE" >/dev/null
docker cp "$KC" "$NAME:/kubeconfig"
docker start "$NAME" >/dev/null

for i in $(seq 1 30); do
  code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 2 "http://127.0.0.1:$PORT/metrics" || true)"
  [ "$code" = 200 ] && break
  [ "$(docker inspect -f '{{.State.Status}}' "$NAME")" = running ] || fail "reloader exited"
  sleep 1
done
[ "$code" = 200 ] || fail "/metrics never answered (last $code)"
# The client's request counter records each failed call to the dead API server, which
# proves the controllers are running and retrying (Reloader does not log those errors).
for i in $(seq 1 30); do
  metrics="$(curl -fsS "http://127.0.0.1:$PORT/metrics")"
  grep -q 'rest_client_requests_total{code="<error>",host="127.0.0.1:1"' <<<"$metrics" && break
  [ "$i" = 30 ] && fail "no failed API requests recorded: the controllers are not trying the API server"
  sleep 1
done
grep -q '^reloader_reload_executed_total' <<<"$metrics" || fail "/metrics carries no reloader_ metrics"
logs="$(docker logs "$NAME" 2>&1)"
for rt in secrets configmaps; do
  grep -q "Starting Controller to watch resource type: $rt" <<<"$logs" || fail "the $rt controller did not start"
done
[ "$(docker inspect -f '{{.State.Status}}' "$NAME")" = running ] || fail "reloader is not running any more"
echo "smoke test passed (uid $user, controllers retrying a dead API server, /metrics serving reloader_ metrics)"
