#!/usr/bin/env bash
# Smoke test for a built dive image. Usage: test.sh <image-ref> [expected-version]
#
# dive analyses a saved image (its own) from a docker-archive in --ci mode, which needs
# no Docker socket: it must read every layer and report the image efficiency.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
WANT="${2:-}"
NAME="quench-dive-smoke-$$"
TAR="$(mktemp)"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -f "$TAR"; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
ver="$(docker run --rm "$IMAGE" --version)"
echo "$ver"
[ -z "$WANT" ] || [ "$ver" = "dive $WANT" ] || { echo "expected dive $WANT"; exit 1; }

docker save "$IMAGE" -o "$TAR"; chmod 0644 "$TAR"
docker create --name "$NAME" -e CI=true "$IMAGE" --ci --source docker-archive /image.tar >/dev/null
docker cp "$TAR" "$NAME:/image.tar"
out="$(docker start -a "$NAME" 2>&1 || true)"
echo "$out" | tail -8
grep -q 'efficiency:' <<<"$out" || { echo "FAIL: dive did not analyse the image"; exit 1; }
grep -q 'Result:PASS' <<<"$out" || { echo "FAIL: dive --ci did not pass on its own image"; exit 1; }
echo "smoke test passed (uid $user, docker-archive analysed)"
