#!/usr/bin/env bash
# Smoke test for a built calico-node-driver-registrar image. Usage: test.sh <image-ref> [version]
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="quench-calico-node-driver-registrar-smoke-$$"
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$WORK"; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "0" ] || [ -z "$user" ] || { echo "expected root like upstream, got '$user'"; exit 1; }

ver="$(docker run --rm "$IMAGE" --version)"
echo "$ver"
# published under Calico's version; the binary reports the upstream registrar release
grep -q "v2\.18\.0" <<<"$ver" || { echo "expected node-driver-registrar v2.18.0"; exit 1; }
# With no driver on the socket the sidecar must keep running and retry the connection. The ones
# that build a Kubernetes client first get a kubeconfig for an API server that is not there.
printf 'apiVersion: v1
kind: Config
clusters: [{name: c, cluster: {server: "https://127.0.0.1:1"}}]
users: [{name: u, user: {token: smoke}}]
contexts: [{name: c, context: {cluster: c, user: u}}]
current-context: c
' > "$WORK/kubeconfig"
chmod -R a+rX "$WORK"
docker run -d --name "$NAME" -v "$WORK:/csi" "$IMAGE" --csi-address=/csi/csi.sock --kubelet-registration-path=/var/lib/kubelet/plugins/smoke/csi.sock --v=5 >/dev/null
sleep 6
[ "$(docker inspect -f '{{.State.Status}}' "$NAME")" = running ] || { echo "csi-node-driver-registrar exited"; docker logs "$NAME" 2>&1 | tail -20; exit 1; }
logs="$(docker logs "$NAME" 2>&1)"
grep -qi 'csi.sock' <<<"$logs" || { echo "no connection attempt to the CSI socket in the log"; tail -20 <<<"$logs"; exit 1; }
docker run --rm --entrypoint /bin/sh "$IMAGE" -c "rm -rf /tmp/x" || { echo "no /bin/sh for the preStop hook"; exit 1; }

echo "smoke test passed (csi-node-driver-registrar ${WANT:-?}; user ${user:-0})"
