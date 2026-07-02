#!/usr/bin/env bash
# Smoke test for a built NSQ image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

# All three daemons must be on PATH and report a stamped version.
# `<bin> -version` prints e.g. "nsqd v1.3.0 (built w/go1.x)".
for bin in nsqd nsqlookupd nsqadmin; do
  echo "$bin version:"
  out="$(docker run --rm --entrypoint "$bin" "$IMAGE" -version 2>&1)"
  echo "$out"
  echo "$out" | grep -qiE "^$bin v[0-9]+\.[0-9]+\.[0-9]+" \
    || { echo "$bin version not stamped"; exit 1; }
done

# must run as the nonroot nsq user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
