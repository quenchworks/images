#!/usr/bin/env bash
# Smoke test for a built Vitess Operator image. Usage: test.sh <image-ref> [version]
# The real test is the chart's kind gate (a VitessCluster reconciled and answering SQL).
# Without a cluster the operator logs its version and exits on the missing kubeconfig.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
out="$(timeout 60 docker run --rm "$IMAGE" 2>&1 || true)"
grep -q "Operator Version: ${WANT}" <<<"$out" || { echo "no 'Operator Version: ${WANT}' in the startup log"; echo "$out" | head -20; exit 1; }
h="$(docker run --rm "$IMAGE" --help 2>&1 || true)"
grep -q -- 'cache_invalidate_interval' <<<"$h" || { echo "unexpected --help output"; echo "$h" | head -20; exit 1; }
echo "smoke test passed (vitess-operator ${WANT:-?}: startup version, CLI, nonroot user $user)"
