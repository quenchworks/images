#!/usr/bin/env bash
# Smoke test for a built cilium-operator image. Usage: test.sh <image-ref> [version]
# Without a cluster the operator cannot run, so this checks the version, then that it
# starts on a read-only root (with /var/run/cilium writable, as the chart mounts it),
# logs its version banner and stops with a fatal config error rather than a panic. Which
# check stops it differs by line (k8s config on 1.20, the cluster-pool CIDR on 1.18/1.19).
# The chart gate runs it as the only operator of a real Cilium in kind.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
ver="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" --version)"
[ -z "$WANT" ] || echo "$ver" | grep -qF "$WANT" || { echo "version mismatch: $ver"; exit 1; }
out="$(timeout 60 docker run --rm --read-only --tmpfs /tmp --tmpfs /var/run/cilium "$IMAGE" --enable-gops=false 2>&1 || true)"
echo "$out" | grep -q 'msg="Cilium Operator"' || { echo "no version banner:"; echo "$out" | tail -20; exit 1; }
echo "$out" | grep -q 'level=fatal' || { echo "did not stop on a config error:"; echo "$out" | tail -20; exit 1; }
if echo "$out" | grep -E 'level=error' | grep -q 'read-only file system'; then echo "wrote outside its volumes:"; echo "$out" | grep 'read-only'; exit 1; fi
if echo "$out" | grep -qE 'panic:|SIGSEGV'; then echo "operator panicked"; echo "$out" | tail -20; exit 1; fi
echo "smoke test passed (cilium-operator ${WANT:-?}, uid $user, stops cleanly without a cluster)"
