#!/usr/bin/env bash
# Smoke test for a built step-issuer image. Usage: test.sh <image-ref> [version]
# step-issuer only works inside a cluster with cert-manager and a step-ca (the chart's kind
# gate issues a real certificate); here the binary must parse its flags as nonroot.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
out="$(docker run --rm "$IMAGE" -help 2>&1)" || { echo "-help failed"; echo "$out"; exit 1; }
grep -q -- '-disable-approval-check' <<<"$out" || { echo "no step-issuer flags in -help"; echo "$out"; exit 1; }
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed (flags parse, nonroot user: $user)"
