#!/usr/bin/env bash
# Smoke test for the Tetragon operator image. Usage: test.sh <image-ref> [version]
#
# The operator has no version command and logs none; the chart's kind gate
# covers what it does (install the CRDs). Here, pointed at a kubeconfig for a
# dead API server (127.0.0.1:1), `serve` must get as far as calling the API
# and fail on the CONNECTION, not on a missing file, library or flag. The
# version argument is accepted and unused: apps/tetragon checks the release
# for both images, which are built in lockstep.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
# Under $HOME, not /tmp: Docker Desktop cannot bind-mount /tmp paths.
work="$(mktemp -d "$HOME/.quench-tetragon-op-test.XXXXXX")"
trap 'rm -rf "$work"' EXIT
cat > "$work/kubeconfig" <<'KC'
apiVersion: v1
kind: Config
clusters: [{name: dead, cluster: {server: "https://127.0.0.1:1", insecure-skip-tls-verify: true}}]
users: [{name: u, user: {token: x}}]
contexts: [{name: c, context: {cluster: dead, user: u}}]
current-context: c
KC
chmod -R a+rX "$work"

help="$(docker run --rm "$IMAGE" --help 2>&1 || true)"
grep -q 'serve' <<<"$help" || { echo "--help did not list serve:"; echo "$help" | head -20; exit 1; }

out="$(timeout 30 docker run --rm -v "$work/kubeconfig:/kc:ro" "$IMAGE" serve --kube-config /kc 2>&1 || true)"
echo "$out" | head -5
grep -q '127.0.0.1:1.*connection refused' <<<"$out" || { echo "serve did not reach the API call"; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed (operator reached the API server, runs as 1001)"
