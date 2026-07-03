#!/usr/bin/env bash
# Smoke test for a built Kyverno image. Usage: test.sh <image-ref>
# The image is shell-free (wolfi-baselayout only), so checks drive the binaries
# directly by overriding the entrypoint.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

# the kubectl-kyverno CLI has a `version` subcommand that prints the stamped
# BuildVersion; the default entrypoint is the admission controller, so override.
echo "kyverno CLI version:"
out="$(docker run --rm --entrypoint /usr/bin/kubectl-kyverno "$IMAGE" version 2>&1)"
echo "$out"
# version must be stamped (built from the tag, not the default "---")
echo "$out" | grep -qiE 'Version:[[:space:]]*v?[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "version not stamped"; exit 1; }

# the main admission controller must at least start and print its help/flags
echo "kyverno controller --help:"
docker run --rm --entrypoint /usr/bin/kyverno "$IMAGE" --help >/dev/null 2>&1 \
  || { echo "kyverno controller did not run"; exit 1; }
echo "controller entrypoint runs"

# must run as the nonroot kyverno user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
