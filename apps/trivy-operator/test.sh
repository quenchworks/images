#!/usr/bin/env bash
# Smoke test for a built trivy-operator image. Usage: test.sh <image-ref> [version]
# The operator needs a Kubernetes API to do anything real; that is the chart's kind
# gate. Here: the binary runs on a read-only root as uid 1001, reports its version, and
# on start fails for want of a cluster rather than crashing.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
set +e
out="$(docker run --rm --read-only --tmpfs /tmp -e OPERATOR_NAMESPACE=default "$IMAGE" 2>&1 | head -40)"
set -e
echo "$out" | tail -12
[ -z "$WANT" ] || echo "$out" | grep -q "\"Version\":\"${WANT}\"" || { echo "expected version $WANT in the start log"; exit 1; }
echo "$out" | grep -qiE 'kubeconfig|in-cluster|KUBERNETES_SERVICE_HOST|config' || { echo "did not reach cluster configuration"; exit 1; }
if echo "$out" | grep -qE '^panic:|SIGSEGV'; then echo "operator panicked"; exit 1; fi
echo "smoke test passed (trivy-operator ${WANT:-?}, uid $user; the chart gate exercises it against a cluster)"
