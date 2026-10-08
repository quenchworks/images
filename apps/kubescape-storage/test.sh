#!/usr/bin/env bash
# Smoke test for a built kubescape-storage image. Usage: test.sh <image-ref> [version]
# These are in-cluster daemons; outside a cluster they exit on missing config. Check each binary
# starts and prints its own output, not an exec failure (missing loader, wrong arch, bad mode).
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
run_bin() {
  local out rc=0
  out="$(timeout 20 docker run --rm --entrypoint "$1" "$IMAGE" --help 2>&1)" || rc=$?
  echo "$1 (exit $rc): $(head -c 300 <<<"$out")"
  [ "$rc" -ne 126 ] && [ "$rc" -ne 127 ] || { echo "$1 did not execute"; exit 1; }
  [ -n "$out" ] || { echo "$1 printed nothing"; exit 1; }
  if grep -qiE 'exec format error|no such file or directory.*exec|permission denied' <<<"$out"; then echo "$1 failed to exec"; exit 1; fi
}
run_bin /usr/bin/storage
run_bin /usr/bin/migration
run_bin /usr/bin/cpexport
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
echo "user: $user"
[ "$user" = "1001" ] || { echo "expected user 1001"; exit 1; }
echo "smoke test passed"
