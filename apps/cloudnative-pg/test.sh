#!/usr/bin/env bash
# Smoke test for a built CloudNativePG image. Usage: test.sh <image-ref> [version]
# Checks the stamped version and runs the step every PostgreSQL pod starts with:
# `manager bootstrap <dest>` copies the binary into a shared volume, and the copy
# must run on its own (the instance pods exec it from there). The chart gate
# runs the operator and a real cluster.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT
chmod 0777 "$WORK"

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

ver="$(docker run --rm "$IMAGE" version)"
echo "$ver"
grep -q "Version:${WANT:-}" <<<"$ver" || { echo "version not stamped as ${WANT:-?}"; exit 1; }

docker run --rm --read-only -v "$WORK:/controller" "$IMAGE" bootstrap /controller/manager
test -x "$WORK/manager" || { echo "bootstrap did not copy an executable manager"; exit 1; }
docker run --rm --read-only -v "$WORK:/controller:ro" --entrypoint /controller/manager "$IMAGE" version \
  | grep -q "Version:${WANT:-}" || { echo "the copied manager does not run"; exit 1; }

echo "smoke test passed (${WANT:-?}: version, bootstrap copy runs; nonroot user: $user)"
