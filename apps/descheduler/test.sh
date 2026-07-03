#!/usr/bin/env bash
# Smoke test for a built descheduler image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

# descheduler exposes its version via the `version` subcommand (there is no
# --version flag on the root command; it prints version.Get() from the
# component-base-style pkg/version).
echo "descheduler version:"
out="$(docker run --rm "$IMAGE" version 2>&1)"
echo "$out"
# version must be stamped from the tag (GitVersion:vX.Y.Z), not left empty
echo "$out" | grep -qiE 'GitVersion:v[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "version not stamped"; exit 1; }

# must run as the nonroot descheduler user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
