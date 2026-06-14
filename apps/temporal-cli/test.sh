#!/usr/bin/env bash
# Smoke test for a built Temporal CLI image. Usage: test.sh <image-ref>
#
# This is a CLI, not a server -- there's nothing to boot and no port to poll. We
# assert the binary runs, reports the pinned version, prints help, and works
# under the hardened runtime contract (nonroot uid 1001, read-only rootfs with a
# writable /tmp tmpfs, HOME=/tmp).
#
# Asserts:
#   - `temporal --version` prints the pinned version
#   - `temporal --help` works
#   - works under --read-only with a writable /tmp tmpfs (a real-world env-config
#     command that writes under $HOME/.config/temporalio succeeds)
#   - the container runs as nonroot uid 1001
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

# Expected pinned version (the melange package version).
EXPECTED="$(awk '/^  version:/{print $2; exit}' "$(dirname "$0")/melange.yaml")"

echo "checking temporal --version prints the pinned tag (${EXPECTED})"
ver="$(docker run --rm "$IMAGE" --version)"
echo "  -> ${ver}"
echo "$ver" | grep -q "${EXPECTED}" || { echo "version mismatch: expected ${EXPECTED}"; exit 1; }

echo "checking temporal --help works"
docker run --rm "$IMAGE" --help >/dev/null

echo "checking it runs under --read-only with a writable /tmp tmpfs (HOME=/tmp)"
# `temporal env set` writes the CLI config under $HOME/.config/temporalio; this
# exercises the read-only-rootfs + writable-HOME contract end to end.
docker run --rm --read-only --tmpfs /tmp:rw "$IMAGE" \
  env set --env smoke --key address --value localhost:7233 >/dev/null
echo "  -> read-only rootfs + writable /tmp OK"

echo "checking the container runs as nonroot uid 1001"
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (version ${EXPECTED}, nonroot uid ${user}, read-only rootfs OK)"
