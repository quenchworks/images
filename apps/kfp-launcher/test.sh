#!/usr/bin/env bash
# Smoke test for a built kfp-launcher image. Usage: test.sh <image-ref> [version]
# Checks the user and that the binary starts and parses its flags. The component itself
# needs a cluster, MySQL and the API server: the kubeflow-pipelines chart gate runs it.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
user="$(docker inspect "$IMAGE" --format "{{.Config.User}}")"
[ "$user" = "1001" ] || { echo "expected user 1001, got $user"; exit 1; }
out="$(docker run --rm --entrypoint /usr/bin/launcher-v2 "$IMAGE" --help 2>&1 || true)"
grep -q -- "-copy" <<<"$out" || { echo "/usr/bin/launcher-v2 --help lacks -copy:"; echo "$out" | head -20; exit 1; }
# Every task pod runs the image as an init container: launcher-v2 --copy <path> copies the
# binary into a shared volume. Do that for real.
out="$(docker run --rm --tmpfs /kfp-launcher:uid=1001,exec --entrypoint /usr/bin/launcher-v2 "$IMAGE" --copy /kfp-launcher/launch 2>&1 || echo FAILED)"
! grep -q FAILED <<<"$out" || { echo "launcher-v2 --copy failed: $out"; exit 1; }
echo "smoke test passed"
