#!/usr/bin/env bash
# Smoke test for a built opa-gatekeeper image. Usage: test.sh <image-ref> [version]
# On a read-only root, with webhook certificates in /certs (generated here) and a
# kubeconfig for an API server that does not answer, the manager must report its stamped
# version, load its certificates, set up its controllers, webhooks and audit, and stop on
# the unreachable API server without a panic. The chart gate enforces a real policy in kind.
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
# here-strings, not echo | grep -q: the log is long, grep -q exits at the first match, echo
# takes SIGPIPE and pipefail turns a match into a failure
grep -q '"starting manager"' <<<"$out" || { echo "manager never started:"; grep '"logger":"setup"' <<<"$out" | tail -10; exit 1; }
[ -z "$WANT" ] || grep -q "gatekeeper/v$WANT" <<<"$out" || { echo "version v$WANT not stamped:"; grep -m1 'user agent' <<<"$out"; exit 1; }
if grep -q 'unable to create client cert watcher' <<<"$out"; then echo "certificates not loaded"; exit 1; fi
# it builds its controllers, webhooks and audit, then stops on the unreachable API server
grep -q '"setting up webhooks"' <<<"$out" && grep -q '"setting up audit"' <<<"$out" || { echo "setup incomplete:"; grep '"logger":"setup"' <<<"$out" | tail -10; exit 1; }
grep -q '127.0.0.1:1' <<<"$out" || { echo "never dialed the configured API server"; exit 1; }
if grep -qE 'panic:|read-only file system' <<<"$out"; then echo "manager crashed or wrote to the root"; exit 1; fi
echo "smoke test passed (opa-gatekeeper ${WANT:-?}, uid $user, certs loaded, full setup, dialed the API server)"
