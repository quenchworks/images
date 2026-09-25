#!/usr/bin/env bash
# Smoke test for a built opa-gatekeeper image. Usage: test.sh <image-ref> [version]
# On a read-only root, with webhook certificates in /certs (generated here) and a
# kubeconfig for an API server that does not answer, the manager must report its stamped
# version, load its certificates, start the manager and its health server, and stay up
# retrying the API server without a panic. The chart gate enforces a real policy in kind.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="gatekeeper-smoke-$$"
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
trap 'docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$WORK"' EXIT
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
mkdir -p "$WORK/certs"
openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj "/CN=gatekeeper-webhook-service.gatekeeper-system.svc" \
  -keyout "$WORK/certs/tls.key" -out "$WORK/certs/tls.crt" 2>/dev/null
cp "$WORK/certs/tls.crt" "$WORK/certs/ca.crt"
cat > "$WORK/kubeconfig" <<'KC'
apiVersion: v1
kind: Config
clusters: [{name: none, cluster: {server: "https://127.0.0.1:1", insecure-skip-tls-verify: true}}]
users: [{name: none, user: {token: none}}]
contexts: [{name: none, context: {cluster: none, user: none}}]
current-context: none
KC
chmod -R a+rX "$WORK"
docker run -d --name "$NAME" --read-only --tmpfs /tmp -v "$WORK/certs:/certs:ro" -v "$WORK/kubeconfig:/etc/kubeconfig:ro" \
  -e KUBECONFIG=/etc/kubeconfig -e POD_NAMESPACE=gatekeeper-system "$IMAGE" --disable-cert-rotation >/dev/null
sleep 15
out="$(docker logs "$NAME" 2>&1)"
echo "$out" | grep -q '"starting manager"' || { echo "manager never started:"; echo "$out" | tail -15; exit 1; }
[ -z "$WANT" ] || echo "$out" | grep -q "gatekeeper/v$WANT" || { echo "version v$WANT not stamped:"; echo "$out" | grep -m1 'user agent'; exit 1; }
if echo "$out" | grep -q 'unable to create client cert watcher'; then echo "certificates not loaded:"; echo "$out" | grep 'cert watcher'; exit 1; fi
[ "$(docker inspect "$NAME" --format '{{.State.Running}}')" = true ] || { echo "manager exited:"; echo "$out" | tail -15; exit 1; }
if echo "$out" | grep -qE 'panic:|read-only file system'; then echo "manager crashed or wrote to the root:"; echo "$out" | tail -15; exit 1; fi
echo "smoke test passed (opa-gatekeeper ${WANT:-?}, uid $user, certs loaded, manager up)"
