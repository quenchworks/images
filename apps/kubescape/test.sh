#!/usr/bin/env bash
# Smoke test for a built kubescape image. Usage: test.sh <image-ref> [version]
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
out="$(docker run --rm "$IMAGE" version 2>&1 || true)"
echo "kubescape version: $out"
[ -z "$WANT" ] || grep -q "v$WANT" <<<"$out" || { echo "expected v$WANT"; exit 1; }
# a real subcommand resolves, not just the root help
docker run --rm "$IMAGE" scan --help >/dev/null 2>&1 || { echo "kubescape scan --help failed"; exit 1; }
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed"
