#!/usr/bin/env bash
# Smoke test for a built oauth2-proxy image. Usage: test.sh <image-ref>
# A full boot needs provider config (client id/secret, cookie secret, upstream),
# so the smoke checks the stamped version + help instead of a live :4180 listen.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

# version must be stamped (built from the tag, not empty/dev). Format:
#   "oauth2-proxy 7.15.3 (built with go1.xx)"
ver="$(docker run --rm "$IMAGE" --version | awk '{print $2}')"
echo "reported version: $ver"
case "$ver" in
  ""|0.0.0|*dev*|*undefined*) echo "version not stamped: '$ver'"; exit 1 ;;
esac

# --help must work (binary is the full reverse-proxy auth server)
docker run --rm "$IMAGE" --help >/dev/null 2>&1 \
  || { echo "oauth2-proxy --help failed"; exit 1; }

# must run as the nonroot oauth2-proxy user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (version $ver, nonroot user: $user)"
