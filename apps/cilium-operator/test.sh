#!/usr/bin/env bash
# Smoke test for a built cilium-operator image. Usage: test.sh <image-ref> [version]
# Without a cluster the operator cannot start, so this checks the version and that it
# builds its object graph on a read-only root and stops exactly on the missing in-cluster
# API config, not on a panic or a missing file.
# The chart gate runs it as the only operator of a real Cilium in kind.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
ver="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" --version)"
[ -z "$WANT" ] || echo "$ver" | grep -qF "$WANT" || { echo "version mismatch: $ver"; exit 1; }
out="$(timeout 60 docker run --rm --read-only --tmpfs /tmp "$IMAGE" --enable-gops=false 2>&1 || true)"
echo "$out" | grep -qE 'unable to load in-cluster configuration|requires k8s to be configured' || { echo "unexpected startup:"; echo "$out" | tail -20; exit 1; }
if echo "$out" | grep -qE 'panic:|SIGSEGV'; then echo "operator panicked"; echo "$out" | tail -20; exit 1; fi
echo "smoke test passed (cilium-operator ${WANT:-?}, uid $user, stops on the missing cluster config)"
