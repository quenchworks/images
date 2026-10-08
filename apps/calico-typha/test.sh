#!/usr/bin/env bash
# Smoke test for a built calico-typha image. Usage: test.sh <image-ref> [version]
# Checks the user and that the binaries report the release. The controllers themselves
# need a cluster: the Calico chart's kind gate runs them.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
out="$(docker run --rm --entrypoint /usr/bin/calico-typha "$IMAGE" --version 2>&1 || true)"
echo "/usr/bin/calico-typha --version: $out"
[ -z "$WANT" ] || grep -q "v$WANT" <<<"$out" || { echo "/usr/bin/calico-typha: expected v$WANT"; exit 1; }
echo "smoke test passed"
