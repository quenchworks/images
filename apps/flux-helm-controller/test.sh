#!/usr/bin/env bash
# Smoke test for a built Flux helm-controller image. Usage: test.sh <image-ref> [version]
# Given a kubeconfig for an API server that does not answer, the controller must start,
# keep retrying that server, and neither panic nor write to its read-only root. The
# chart gate reconciles real Flux objects in kind.
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
out="$(timeout 25 docker run --rm --read-only --tmpfs /tmp -e KUBECONFIG=/etc/kubeconfig \
  -v "$WORK/kubeconfig:/etc/kubeconfig:ro" "$IMAGE"  2>&1 || true)"
grep -q '127.0.0.1:1' <<<"$out" || { echo "never dialed the configured API server:"; tail -15 <<<"$out"; exit 1; }
if grep -qE 'panic:|read-only file system' <<<"$out"; then echo "crashed or wrote to the root:"; tail -15 <<<"$out"; exit 1; fi
echo "  started, dialed 127.0.0.1:1, no panic, nothing written to the root"

echo "smoke test passed (Flux helm-controller, uid $user, dead-API boot on a read-only root)"
