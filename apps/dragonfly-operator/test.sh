#!/usr/bin/env bash
# Smoke test for a built Dragonfly Operator image. Usage: test.sh <image-ref> [version]
# The real test is the chart's kind gate (a Dragonfly resource reconciled and answering Redis commands).
# Upstream's Version is a const ("source"), so the version comes from the image label.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
label="$(docker inspect "$IMAGE" --format '{{index .Config.Labels "org.opencontainers.image.version"}}')"
[ -z "$WANT" ] || [ "$label" = "$WANT" ] || { echo "image label $label, expected $WANT"; exit 1; }
v="$(docker run --rm "$IMAGE" -version 2>&1)"; [ "$v" = source ] || { echo "unexpected -version output: $v"; exit 1; }
h="$(docker run --rm "$IMAGE" --help 2>&1 || true)"
grep -q -- '-dragonfly-image' <<<"$h" || { echo "unexpected --help output"; echo "$h" | head -20; exit 1; }
echo "smoke test passed (dragonfly-operator ${WANT:-?}: CLI, image label, nonroot user $user)"
