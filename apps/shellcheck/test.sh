#!/usr/bin/env bash
# Smoke test for a built shellcheck image. Usage: test.sh <image-ref> [version]
# Lints a known-bad input from stdin and requires the expected rule codes.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
T="$(mktemp)"
trap 'rm -f "$T"' EXIT
printf '#!/bin/sh\necho $1\n' | docker run --rm -i "$IMAGE" -f gcc - > "$T" 2>&1 || true
grep -q "SC2086" "$T" || { echo "SC2086 not reported:"; cat "$T"; exit 1; }
v="$(docker run --rm "$IMAGE" --version | awk '/^version:/ {print $2}')"
echo "reported version: $v"
[ -n "$v" ] || { echo "no version"; exit 1; }
[ -z "$WANT" ] || [ "$v" = "$WANT" ] || { echo "expected $WANT"; exit 1; }
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed (shellcheck $v, nonroot user: $user, known-bad input flagged)"
