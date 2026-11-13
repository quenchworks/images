#!/usr/bin/env bash
# Smoke test for a built Redis Operator image. Usage: test.sh <image-ref> [version]
# The real test is the chart's kind gate (Redis resources reconciled and answering commands).
# Upstream stamps no version into the binary, so the version comes from the image label.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
label="$(docker inspect "$IMAGE" --format '{{index .Config.Labels "org.opencontainers.image.version"}}')"
[ -z "$WANT" ] || [ "$label" = "$WANT" ] || { echo "image label $label, expected $WANT"; exit 1; }
h="$(docker run --rm "$IMAGE" --help 2>&1 || true)"
grep -q -- '--feature-gates' <<<"$h" || { echo "unexpected manager --help output"; echo "$h" | head -20; exit 1; }
# the init container runs the same image as "/operator agent bootstrap"
a="$(docker run --rm --entrypoint /operator "$IMAGE" agent bootstrap --help 2>&1 || true)"
grep -q -- '--sentinel' <<<"$a" || { echo "unexpected agent bootstrap --help output"; echo "$a" | head -20; exit 1; }
echo "smoke test passed (redis-operator ${WANT:-?}: manager and agent CLI, image label, nonroot user $user)"
