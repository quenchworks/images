#!/usr/bin/env bash
# Smoke test for a built calico-key-cert-provisioner image. Usage: test.sh <image-ref> [version]
# The binary only works inside a pod (it files a CSR with the API server), so check the user
# and that it starts and refuses to run without its configuration.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
out="$(docker run --rm "$IMAGE" 2>&1 || true)"
echo "$out" | head -10
[ -n "$out" ] || { echo "no output from key-cert-provisioner"; exit 1; }
! grep -qiE 'exec format|not found|no such file' <<<"$out" || { echo "binary did not start"; exit 1; }
echo "smoke test passed"
