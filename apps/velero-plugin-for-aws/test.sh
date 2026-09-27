#!/usr/bin/env bash
# Smoke test for a built Velero AWS plugin image. Usage: test.sh <image-ref> [version]
# Runs the image as Velero runs it, an init container with the plugins volume at
# /target, and requires the copied plugin to be an executable Velero plugin.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

chmod 0777 "$WORK"
docker run --rm --read-only -v "$WORK:/target" "$IMAGE"
[ -x "$WORK/velero-plugin-for-aws" ] || { echo "plugin not copied to /target"; ls -la "$WORK"; exit 1; }
echo "  plugin copied to /target"
out="$("$WORK/velero-plugin-for-aws" 2>&1 || true)"
grep -q "This binary is a plugin" <<<"$out" || { echo "copied file is not the Velero plugin: $out"; exit 1; }
echo "  copied binary is the Velero plugin"

echo "smoke test passed (velero-plugin-for-aws ${2:-?}: init copy; nonroot user: $user)"
