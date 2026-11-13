#!/usr/bin/env bash
# Smoke test for a built Prometheus Operator image. Usage: test.sh <image-ref> [version]
# The operator needs a Kubernetes API to do anything, so its real test is the chart's kind gate
# (a Prometheus resource reconciled into a running StatefulSet). Here: version, CLI, nonroot user.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
v="$(docker run --rm "$IMAGE" --version 2>&1)"; echo "$v" | head -2
[ -z "$WANT" ] || grep -qF "$WANT" <<<"$v" || { echo "expected $WANT"; exit 1; }
h="$(docker run --rm "$IMAGE" --help 2>&1 || true)"
# Go's flag package prints one dash: -prometheus-config-reloader
grep -q -- '-prometheus-config-reloader' <<<"$h" || { echo "unexpected --help output"; echo "$h" | head -20; exit 1; }
# the CRDs are compiled into the binary; the chart ships the same set
crds="$(docker run --rm "$IMAGE" crds)"
n="$(grep -c '^kind: CustomResourceDefinition' <<<"$crds" || true)"
[ "$n" -ge 8 ] || { echo "crds printed $n CustomResourceDefinitions"; exit 1; }
grep -q 'name: prometheuses.monitoring.coreos.com' <<<"$crds" || { echo "Prometheus CRD missing"; exit 1; }
echo "smoke test passed (prometheus-operator ${WANT:-?}: version, CLI, $n CRDs embedded, nonroot user $user)"
