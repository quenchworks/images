#!/usr/bin/env bash
# Smoke test for a built KubeRay operator image. Usage: test.sh <image-ref> [version]
# Without a cluster the operator cannot run, so this proves what it can alone: it
# starts, loads its feature gates, builds the manager and then fails on the API
# server named in a kubeconfig pointing at 127.0.0.1:1. That exact failure means
# every step before contacting the API worked. The chart gate runs it for real.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
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

out="$(timeout 60 docker run --rm --read-only -v "$WORK:/w:ro" -e KUBECONFIG=/w/kubeconfig "$IMAGE" 2>&1 || true)"
echo "$out" | cut -c1-200 | head -8
grep -q '"msg":"Loaded feature gates"' <<<"$out" || { echo "operator did not load its feature gates"; exit 1; }
grep -q '"msg":"Setup manager"' <<<"$out" || { echo "operator did not reach manager setup"; exit 1; }
grep -q '127.0.0.1:1' <<<"$out" || { echo "operator did not reach the API server step"; exit 1; }

echo "smoke test passed (starts, loads feature gates, reaches the API; nonroot user: $user)"
