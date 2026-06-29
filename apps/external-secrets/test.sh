#!/usr/bin/env bash
# Smoke test for a built External Secrets Operator image. Usage: test.sh <image-ref>
# The controller manager needs in-cluster credentials to actually run, so the real
# runtime test is the chart's kind install gate. Here we verify the binary executes,
# exposes the operator CLI, and that the image runs as the nonroot user.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

echo "checking the CLI responds"
# --help exits 0 and prints the operator description (the manager subcommand tree).
out="$(docker run --rm "$IMAGE" --help 2>&1)" || { echo "--help failed"; echo "$out"; exit 1; }
echo "$out" | grep -qi "reconciles ExternalSecrets\|external-secrets" \
  || { echo "unexpected --help output"; echo "$out"; exit 1; }

echo "checking subcommands are present (webhook, certcontroller)"
echo "$out" | grep -qi "webhook" || { echo "missing webhook subcommand"; echo "$out"; exit 1; }
echo "$out" | grep -qi "certcontroller" || { echo "missing certcontroller subcommand"; echo "$out"; exit 1; }

# must run as the nonroot external-secrets user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (CLI responds, nonroot user: $user)"
