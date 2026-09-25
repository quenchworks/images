#!/usr/bin/env bash
# Smoke test for a built argo-rollouts image. Usage: test.sh <image-ref> [version]
# Given a kubeconfig for an API server that does not answer, the controller must log its
# stamped version and stop on the failed API discovery against that server, on a
# read-only root and without a panic. The chart gate runs a real canary in kind.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
cat > "$WORK/kubeconfig" <<'KC'
apiVersion: v1
kind: Config
clusters: [{name: none, cluster: {server: "https://127.0.0.1:1", insecure-skip-tls-verify: true}}]
users: [{name: none, user: {token: none}}]
contexts: [{name: none, context: {cluster: none, user: none}}]
current-context: none
KC
chmod -R a+rX "$WORK"
out="$(timeout 60 docker run --rm --read-only --tmpfs /tmp -v "$WORK/kubeconfig:/etc/kubeconfig:ro" "$IMAGE" --kubeconfig /etc/kubeconfig 2>&1 || true)"
echo "$out" | grep -q 'Argo Rollouts controller starting' || { echo "no startup line:"; echo "$out" | tail -10; exit 1; }
[ -z "$WANT" ] || echo "$out" | grep -q "version=v$WANT" || { echo "version v$WANT not stamped:"; echo "$out" | head -3; exit 1; }
echo "$out" | grep -q '127.0.0.1:1/version' || { echo "never dialed the configured API server:"; echo "$out" | tail -10; exit 1; }
if echo "$out" | grep -qE 'panic:|read-only file system'; then echo "controller crashed or wrote to the root:"; echo "$out" | tail -10; exit 1; fi
echo "smoke test passed (argo-rollouts ${WANT:-?}, uid $user, version stamped, dialed the API server)"
