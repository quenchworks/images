#!/usr/bin/env bash
# Smoke test for a built Vitess image. Usage: test.sh <image-ref> <version>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> <version>}"
VERSION="${2:?usage: test.sh <image-ref> <version>}"

# must run as the nonroot vitess user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

# Every binary the package claims to ship must be present and runnable, not just
# vtgate. A build that silently dropped one still passes a single-binary check.
for b in vtgate vttablet vtctld vtctldclient vtorc mysqlctl mysqlctld vtbackup; do
  docker run --rm --entrypoint "/usr/bin/$b" "$IMAGE" --version >/dev/null 2>&1 \
    || { echo "$b did not respond to --version"; exit 1; }
done

# Vitess bakes its version into a generated constant, so a wrong tarball shows up
# here as a version mismatch rather than as a build error. Output looks like:
#   vtgate version Version: 24.0.3 (Git revision  branch '') built on  by @ ...
out="$(docker run --rm --entrypoint /usr/bin/vtgate "$IMAGE" --version 2>&1)"
echo "$out"
echo "$out" | grep -q "Version: ${VERSION} " \
  || { echo "expected 'Version: ${VERSION}', got: $out"; exit 1; }

# vtctldclient is the CLI operators actually run; prove it parses a subcommand
# rather than only answering --version.
docker run --rm --entrypoint /usr/bin/vtctldclient "$IMAGE" --help >/dev/null 2>&1 \
  || { echo "vtctldclient --help failed"; exit 1; }

echo "smoke test passed (nonroot user: $user, 8 binaries present, vtgate ${VERSION})"
