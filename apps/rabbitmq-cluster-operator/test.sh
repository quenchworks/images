#!/usr/bin/env bash
# Smoke test for a built RabbitMQ Cluster Operator image. Usage: test.sh <image-ref> [version]
# The real test is the chart's kind gate (a RabbitmqCluster reconciled and serving AMQP). Here:
# nonroot user, the CLI, and its first startup check (OPERATOR_NAMESPACE) failing cleanly.
# Upstream stamps no version into the binary; the version is the image label.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
label="$(docker inspect "$IMAGE" --format '{{index .Config.Labels "org.opencontainers.image.version"}}')"
[ -z "$WANT" ] || [ "$label" = "$WANT" ] || { echo "image label $label, expected $WANT"; exit 1; }
h="$(docker run --rm "$IMAGE" --help 2>&1 || true)"
grep -q -- '-metrics-bind-address' <<<"$h" || { echo "unexpected --help output"; echo "$h" | head -20; exit 1; }
out="$(docker run --rm "$IMAGE" 2>&1 || true)"
grep -q 'unable to find operator namespace' <<<"$out" || { echo "unexpected startup output:"; echo "$out" | head -20; exit 1; }
echo "smoke test passed (rabbitmq-cluster-operator ${WANT:-?}: CLI, startup check, nonroot user $user)"
