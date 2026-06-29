#!/usr/bin/env bash
# Smoke test for a built cert-manager controller image. Usage: test.sh <image-ref>
# The component only does real work inside a cluster (the chart's kind install gate is
# the runtime test); here we verify the binary executes its CLI and runs as nonroot.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref>}"

echo "checking the controller CLI responds"
out="$(docker run --rm "$IMAGE" --help 2>&1)" || { echo "--help failed"; echo "$out"; exit 1; }
echo "$out" | grep -qi "Usage:" || { echo "no usage in --help"; echo "$out"; exit 1; }
echo "$out" | grep -qi "controller" || { echo "missing 'controller' in --help"; echo "$out"; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed (controller CLI responds, nonroot user: $user)"
