#!/usr/bin/env bash
# Smoke test for a built opentelemetry-operator image. Usage: test.sh <image-ref> [version]
# Given a kubeconfig for an API server that does not answer and its NAMESPACE, the manager
# must log its stamped version and default component images with real tags (versions.txt
# stamped in, not 0.0.0), dial that server, and stop without a panic on a read-only root
# (it needs the API to set up its NetworkPolicies). The chart gate runs a collector in kind.
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
out="$(timeout 60 docker run --rm --read-only --tmpfs /tmp -v "$WORK/kubeconfig:/etc/kubeconfig:ro" \
  -e KUBECONFIG=/etc/kubeconfig -e NAMESPACE=opentelemetry-operator-system "$IMAGE" 2>&1 || true)"
start="$(grep -m1 'Starting the OpenTelemetry Operator' <<<"$out" || true)"
[ -n "$start" ] || { echo "no start line:"; tail -10 <<<"$out"; exit 1; }
[ -z "$WANT" ] || grep -q "\"opentelemetry-operator\":\"$WANT\"" <<<"$start" || { echo "version $WANT not stamped:"; head -c 300 <<<"$start"; exit 1; }
grep -q '"collector-image":"ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector:0\.[1-9]' <<<"$start" \
  || { echo "default component versions not stamped:"; grep -o '"collector-image":"[^"]*"' <<<"$start"; exit 1; }
grep -q '127.0.0.1:1' <<<"$out" || { echo "never dialed the configured API server"; tail -10 <<<"$out"; exit 1; }
if grep -qE 'panic:|read-only file system' <<<"$out"; then echo "operator crashed or wrote to the root"; tail -10 <<<"$out"; exit 1; fi
echo "smoke test passed (opentelemetry-operator ${WANT:-?}, uid $user, versions stamped, dialed the API server)"
