#!/usr/bin/env bash
# Smoke test for a built KServe controller image. Usage: test.sh <image-ref> [version]
# The manager carries no version flag, so this checks it runs as uid 1001 on a
# read-only root and exposes its flags. The chart gate runs it against a real
# cluster and an InferenceService.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

help="$(docker run --rm --read-only "$IMAGE" --help 2>&1 || true)"
for flag in health-probe-addr metrics-addr leader-elect; do
  grep -q -- "-$flag" <<<"$help" || { echo "flag -$flag missing:"; echo "$help"; exit 1; }
done
echo "manager flags present"

echo "smoke test passed (kserve-controller ${2:-?}; nonroot user: $user)"
