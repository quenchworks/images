#!/usr/bin/env bash
# Smoke test for a built rqlite image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

# rqlited (server) -- version must be stamped from the tag (default is bare "10").
echo "rqlited version:"
out="$(docker run --rm --entrypoint /usr/bin/rqlited "$IMAGE" -version 2>&1)"
echo "$out"
echo "$out" | grep -qiE '[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "rqlited version not stamped"; exit 1; }

# rqlite (CLI) -- also stamped. The CLI's flag parser reads "-version" as bundled
# short flags, so its version flag is "-v" (or "--version").
echo "rqlite (CLI) version:"
cli="$(docker run --rm --entrypoint /usr/bin/rqlite "$IMAGE" -v 2>&1)"
echo "$cli"
echo "$cli" | grep -qiE '[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "rqlite CLI version not stamped"; exit 1; }

# must run as the nonroot rqlite user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
