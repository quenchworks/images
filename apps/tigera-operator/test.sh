#!/usr/bin/env bash
# Smoke test for a built tigera-operator image. Usage: test.sh <image-ref> [version]
# Checks the user, the version, and that the operator's built-in image list is Calico
# v3.32.0, the release the calico-* images are built from. The operator itself needs a
# cluster: the Calico chart's kind gate runs it.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
out="$(docker run --rm "$IMAGE" -version 2>&1)"
echo "$out"
[ -z "$WANT" ] || grep -q "v$WANT" <<<"$out" || { echo "expected operator v$WANT"; exit 1; }
imgs="$(docker run --rm "$IMAGE" -print-images list 2>&1)"
echo "$imgs" | head -20
grep -q 'calico/node:v3.32.0' <<<"$imgs" || { echo "operator does not deploy calico/node:v3.32.0"; exit 1; }
echo "smoke test passed"
