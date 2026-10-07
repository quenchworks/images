#!/usr/bin/env bash
# Smoke test for a built ceph-csi-operator image. Usage: test.sh <image-ref> [version]
# The manager needs a cluster; without a kubeconfig it must start, read its flags and
# fail on the missing API server, which shows the binary and its flag set are intact.
# The rook-ceph chart gate exercises it for real.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
# the manager panics without OPERATOR_NAMESPACE, even for --help
help="$(docker run --rm -e OPERATOR_NAMESPACE=smoke "$IMAGE" --help 2>&1 || true)"
grep -q 'leader-elect' <<<"$help" || { echo "manager --help lacks its flags: ${help:0:400}"; exit 1; }
out="$(docker run --rm -e OPERATOR_NAMESPACE=smoke "$IMAGE" 2>&1 || true)"
grep -qiE 'kubeconfig|in-cluster|KUBERNETES_SERVICE_HOST|unable to get kubeconfig' <<<"$out" || { echo "unexpected start: ${out:0:400}"; exit 1; }
echo "manager runs and stops on the missing API server"
echo "smoke test passed"
