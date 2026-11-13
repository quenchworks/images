#!/usr/bin/env bash
# Smoke test for a built Kueue image. Usage: test.sh <image-ref> [version]
# The operator needs a Kubernetes API to do anything, so its real test is the chart's kind gate
# (a ClusterQueue admitting a Job).
# Here: nonroot user, the CLI, and the version stamped into the startup log.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
h="$(docker run --rm "$IMAGE" --help 2>&1 || true)"
grep -q -- '-config' <<<"$h" || { echo "unexpected --help output"; echo "$h" | head -20; exit 1; }
# No API server: it logs its version, then fails to load a kubeconfig and exits non-zero.
out="$(docker run --rm "$IMAGE" 2>&1 || true)"
[ -z "$WANT" ] || grep -qF "v$WANT" <<<"$out" || { echo "version v$WANT not in startup log:"; echo "$out" | head -20; exit 1; }
echo "smoke test passed (kueue ${WANT:-?}: CLI, version in startup log, nonroot user $user)"
