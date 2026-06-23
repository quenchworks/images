#!/usr/bin/env bash
# Smoke test for a built JMeter image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

echo "jmeter --version:"
out="$(docker run --rm "$IMAGE" --version 2>&1)"
echo "$out"
# the JMeter banner must print and report a version (proves the launcher found
# the JRE and the jars, not just that the entrypoint exists)
echo "$out" | grep -qiE '[0-9]+\.[0-9]+' \
  || { echo "no version in output"; exit 1; }
echo "$out" | grep -qiE 'apache jmeter|copyright' \
  || { echo "JMeter banner not printed"; exit 1; }

# must run as the nonroot jmeter user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
