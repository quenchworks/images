#!/usr/bin/env bash
# Smoke test for a built csi-livenessprobe image. Usage: test.sh <image-ref> [version]
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="quench-csi-livenessprobe-smoke-$$"
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$WORK"; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "0" ] || [ -z "$user" ] || { echo "expected root like upstream, got '$user'"; exit 1; }

# livenessprobe has no --version flag: check the stamp in the binary.
cid="$(docker create "$IMAGE")"; docker cp "$cid:/livenessprobe" "$WORK/bin" >/dev/null; docker rm "$cid" >/dev/null
[ -z "$WANT" ] || grep -aq "v$WANT" "$WORK/bin" || { echo "binary not stamped v$WANT"; exit 1; }
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
docker run -d --name "$NAME" -v "$WORK:/csi" "$IMAGE" --csi-address=/csi/csi.sock --v=5 >/dev/null
sleep 6
[ "$(docker inspect -f '{{.State.Status}}' "$NAME")" = running ] || { echo "livenessprobe exited"; docker logs "$NAME" 2>&1 | tail -20; exit 1; }
logs="$(docker logs "$NAME" 2>&1)"
grep -qi 'csi.sock' <<<"$logs" || { echo "no connection attempt to the CSI socket in the log"; tail -20 <<<"$logs"; exit 1; }

echo "smoke test passed (csi-livenessprobe ${WANT:-?}; user ${user:-0})"
