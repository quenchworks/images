#!/usr/bin/env bash
# Smoke test for a built ingress-nginx controller image. Usage: test.sh <image-ref>
# The controller only does real work inside a cluster (the chart's kind ingress gate is
# the runtime test); here we verify the binary reports its version and runs nonroot.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref>}"
echo "nginx-ingress-controller --version"
out="$(docker run --rm "$IMAGE" --version 2>&1)"; echo "$out" | head -5
echo "$out" | grep -qiE "Release|Controller|nginx" || { echo "no version banner"; echo "$out"; exit 1; }
ver="$(echo "$out" | grep -oE "v?[0-9]+\.[0-9]+\.[0-9]+" | head -1)"
[ -n "$ver" ] || { echo "version not reported"; exit 1; }
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed (ingress-nginx $ver, nonroot $user)"
