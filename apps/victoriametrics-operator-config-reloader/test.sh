#!/usr/bin/env bash
# Smoke test for the VictoriaMetrics Operator config-reloader image. Usage: test.sh <image-ref> [version]
# The real test is the chart's kind gate (a VMSingle reconciled and queried). Here: nonroot user and
# the version stamped by the build.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
v="$(docker run --rm "$IMAGE" -version 2>&1)"; echo "$v" | head -2
[ -z "$WANT" ] || grep -qF "v$WANT" <<<"$v" || { echo "expected v$WANT"; exit 1; }
echo "smoke test passed (vm-config-reloader ${WANT:-?}: version, nonroot user $user)"
