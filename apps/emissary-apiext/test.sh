#!/usr/bin/env bash
# Smoke test for a built Emissary-ingress apiext image. Usage: test.sh <image-ref> [version]
# Outside a cluster apiext stops at its in-cluster config, after logging its start line with
# the version it read from ambassador.version; that is what can be checked here.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
out="$(docker run --rm "$IMAGE" 2>&1 || true)"
echo "$out" | tail -4
grep -q 'starting Emissary-ingress apiext webhook conversion server' <<<"$out" \
  || { echo "no start line"; exit 1; }
ver="$(grep -oE '"version": ?"[^"]+"' <<<"$out" | head -1 | grep -oE '[0-9]+\.[0-9]+\.[0-9]+[^"]*')"
case "$ver" in ""|MISSING*) echo "version not read: '$ver'"; exit 1 ;; esac
[ -z "${2:-}" ] || [ "$ver" = "$2" ] || { echo "version $ver, expected $2"; exit 1; }
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "8888" ] || { echo "expected user 8888, got '$user'"; exit 1; }
echo "smoke test passed (version $ver, user $user)"
