#!/usr/bin/env bash
# Smoke test for a built KEDA image. Usage: test.sh <image-ref>
# The operator has no --version subcommand (it logs the version at startup once
# it reaches a cluster), so we prove the binary runs by parsing its flags via
# --help, which pflag prints then exits 0. We also confirm the metrics adapter
# binary runs and that the image is nonroot (uid 1001).
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

echo "operator (keda) --help:"
out="$(docker run --rm "$IMAGE" --help 2>&1 || true)"
echo "$out" | head -20
# a KEDA-operator-specific flag proves this is the operator binary and it ran
echo "$out" | grep -qi -- '--leader-elect' \
  || { echo "operator did not print its flags"; exit 1; }

echo "adapter (keda-adapter) --help:"
aout="$(docker run --rm --entrypoint /usr/bin/keda-adapter "$IMAGE" --help 2>&1 || true)"
echo "$aout" | head -20
echo "$aout" | grep -qiE 'usage|--secure-port|--help' \
  || { echo "adapter did not print its flags"; exit 1; }

# must run as the nonroot keda user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user; operator + adapter both run)"
