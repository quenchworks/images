#!/usr/bin/env bash
# Smoke test for a built Node Feature Discovery image. Usage: test.sh <image-ref> [version]
# The real test is the chart's kind gate (workers label the node, a NodeFeatureRule adds a label).
# Here: nonroot user, and every shipped binary runs and reports the release version.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
for bin in nfd-master nfd-worker nfd-topology-updater nfd-gc; do
  v="$(docker run --rm --entrypoint "/usr/bin/$bin" "$IMAGE" -version 2>&1)"
  [ -z "$WANT" ] || grep -qF "v$WANT" <<<"$v" || { echo "$bin: expected v$WANT, got: $v"; exit 1; }
done
echo "smoke test passed (node-feature-discovery ${WANT:-?}: 4 binaries report the version, nonroot user $user)"
