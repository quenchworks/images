#!/usr/bin/env bash
# Smoke test for a built cilium-operator image. Usage: test.sh <image-ref> [version]
# The operator is given a kubeconfig for an API server that does not answer (with no
# config at all, upstream 1.19.8 nil-derefs in rest_config_provider.go, a path a pod
# never takes). On a read-only root, with /var/run/cilium writable as the chart mounts
# it, it must log its version banner, keep retrying that server and stay up without a
# panic. The chart gate runs it as the only operator of a real Cilium in kind.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="cilium-operator-smoke-$$"
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
trap 'docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$WORK"' EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
ver="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" --version)"
[ -z "$WANT" ] || echo "$ver" | grep -qF "$WANT" || { echo "version mismatch: $ver"; exit 1; }

cat > "$WORK/kubeconfig" <<'KC'
apiVersion: v1
kind: Config
clusters: [{name: none, cluster: {server: "https://127.0.0.1:1", insecure-skip-tls-verify: true}}]
users: [{name: none, user: {token: none}}]
contexts: [{name: none, context: {cluster: none, user: none}}]
current-context: none
KC
chmod -R a+rX "$WORK"
docker run -d --name "$NAME" --read-only --tmpfs /tmp --tmpfs /var/run/cilium \
  -v "$WORK/kubeconfig:/etc/kubeconfig:ro" "$IMAGE" --enable-gops=false --k8s-kubeconfig-path=/etc/kubeconfig >/dev/null
sleep 20
out="$(docker logs "$NAME" 2>&1)"
[ "$(docker inspect "$NAME" --format '{{.State.Running}}')" = true ] || { echo "operator exited:"; echo "$out" | tail -20; exit 1; }
echo "$out" | grep -q 'msg="Cilium Operator"' || { echo "no version banner:"; echo "$out" | tail -20; exit 1; }
echo "$out" | grep -q '127.0.0.1:1' || { echo "never dialed the configured API server:"; echo "$out" | tail -20; exit 1; }
if echo "$out" | grep -qE 'panic:|SIGSEGV'; then echo "operator panicked"; echo "$out" | tail -20; exit 1; fi
if echo "$out" | grep -q 'read-only file system'; then echo "wrote outside its volumes:"; echo "$out" | grep 'read-only'; exit 1; fi
echo "smoke test passed (cilium-operator ${WANT:-?}, uid $user, up and retrying the API server)"
