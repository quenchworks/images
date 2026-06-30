#!/usr/bin/env bash
# Smoke test for a built Envoy image. Usage: test.sh <image-ref>
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref>}"
echo "envoy --version (stamped, not 0.0.0)"
out="$(docker run --rm "$IMAGE" --version 2>&1)"; echo "  $out"
echo "$out" | grep -qiE "envoy version|/[0-9]+\.[0-9]+\.[0-9]+/" || { echo "no version banner"; exit 1; }
ver="$(echo "$out" | grep -oE "[0-9]+\.[0-9]+\.[0-9]+" | head -1)"
case "$ver" in ""|0.0.0) echo "version not stamped: '$ver'"; exit 1 ;; esac
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed (envoy $ver, nonroot $user)"
