#!/usr/bin/env bash
# Smoke test for a built Cadence image. Usage: test.sh <image-ref>
# Cadence needs an external datastore (Cassandra/MySQL) to actually start, so the
# smoke test only exercises the version banners of both bundled binaries (which
# run standalone) and confirms the image is nonroot.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

# The entrypoint is cadence-server, so --version is passed straight to it.
echo "cadence-server --version:"
srv="$(docker run --rm "$IMAGE" --version 2>&1)"
echo "$srv"
# ReleaseVersion is stamped from the tag (not the default "unknown")
echo "$srv" | grep -qiE 'Release version:[[:space:]]*[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "server version not stamped"; exit 1; }

# The CLI is the second binary; override the entrypoint to exercise it.
echo "cadence --version:"
cli="$(docker run --rm --entrypoint /usr/bin/cadence "$IMAGE" --version 2>&1)"
echo "$cli"
echo "$cli" | grep -qiE 'Release version:[[:space:]]*[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "cli version not stamped"; exit 1; }

# must run as the nonroot cadence user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
