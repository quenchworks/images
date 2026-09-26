#!/usr/bin/env bash
# Smoke test for a built hadolint image. Usage: test.sh <image-ref> [version]
# Lints a known-bad input from stdin and requires the expected rule codes.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
T="$(mktemp)"
trap 'rm -f "$T"' EXIT
printf 'FROM ubuntu\nRUN apt-get update && apt-get install -y curl\n' | docker run --rm -i "$IMAGE" --no-color - > "$T" 2>&1 || true
grep -q "DL3006" "$T" && grep -q "DL3008" "$T" || { echo "DL3006/DL3008 not reported:"; cat "$T"; exit 1; }
v="$(docker run --rm "$IMAGE" --version | awk '{print $NF}')"
echo "reported version: $v"
[ -n "$v" ] || { echo "no version"; exit 1; }
[ -z "$WANT" ] || [ "$v" = "$WANT" ] || { echo "expected $WANT"; exit 1; }
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed (hadolint $v, nonroot user: $user, known-bad input flagged)"
