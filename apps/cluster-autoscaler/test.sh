#!/usr/bin/env bash
# Smoke test for a built Cluster Autoscaler image. Usage: test.sh <image-ref> [version]
# Without a reachable cluster the autoscaler exits after its first API call, so this
# proves the binary starts, stamps its version, loads the kwok provider and reaches the
# API server. Scaling itself is gated by the chart's kind test.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
# The kubeconfig lives beside this script: Docker Desktop cannot mount /tmp here.
K="$(mktemp -d -p "$(cd "$(dirname "$0")" && pwd)")"
trap 'rm -rf "$K"' EXIT
cat > "$K/config" <<'KC'
apiVersion: v1
kind: Config
clusters: [{name: dead, cluster: {server: "https://127.0.0.1:1"}}]
users: [{name: u, user: {token: x}}]
contexts: [{name: c, context: {cluster: dead, user: u}}]
current-context: c
KC
chmod -R a+rX "$K"

out="$(docker run --rm -v "$K:/kc:ro" "$IMAGE" /cluster-autoscaler --v=1 \
  --cloud-provider=kwok --kubeconfig=/kc/config 2>&1 || true)"
echo "$out" | tail -5
ver="$(grep -oE 'Cluster Autoscaler [0-9]+\.[0-9]+\.[0-9]+' <<<"$out" | awk '{print $3}')"
[ -n "$ver" ] || { echo "no version line"; exit 1; }
[ -z "${2:-}" ] || [ "$ver" = "$2" ] || { echo "version $ver, expected $2"; exit 1; }
grep -q 'Failed to get nodes from apiserver: .*127.0.0.1:1' <<<"$out" \
  || { echo "did not reach the API server call"; exit 1; }

# the image matches upstream's layout: CMD only, binary at /cluster-autoscaler
ep="$(docker inspect "$IMAGE" --format '{{json .Config.Entrypoint}}')"
case "$ep" in null|"[]") ;; *) echo "unexpected ENTRYPOINT $ep"; exit 1 ;; esac
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed (version $ver, user $user)"
