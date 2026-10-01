#!/usr/bin/env bash
# Smoke test for a built ko image. Usage: test.sh <image-ref> [expected-version]
#
# A real `ko build`: a two-file Go module goes in with docker cp, ko compiles it with the
# image's own go toolchain onto ko's default base and writes an image tarball, which is
# then loaded and run. Needs network (the base image and the module proxy).
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
WANT="${2:-}"
NAME="quench-ko-smoke-$$"
SRC="$(mktemp -d)"; OUT="$(mktemp -d)"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$SRC" "$OUT"; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
ver="$(docker run --rm "$IMAGE" version)"
echo "ko $ver"
[ -z "$WANT" ] || [ "$ver" = "$WANT" ] || { echo "expected $WANT"; exit 1; }

mkdir -p "$SRC/app"
printf 'module example.com/smoke\n\ngo 1.22\n' > "$SRC/app/go.mod"
printf 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("built by ko") }\n' > "$SRC/app/main.go"
chmod -R a+rwX "$SRC"
case "$(uname -m)" in aarch64|arm64) plat=linux/arm64 ;; *) plat=linux/amd64 ;; esac
docker create --name "$NAME" -w /tmp/app -e KO_DOCKER_REPO=example.com/smoke \
  "$IMAGE" build --push=false --bare --tarball=/tmp/out.tar --platform="$plat" . >/dev/null
docker cp "$SRC/app/." "$NAME:/tmp/app"
docker start -a "$NAME" 2>&1 | tail -5
docker cp "$NAME:/tmp/out.tar" "$OUT/out.tar"
ref="$(docker load -i "$OUT/out.tar" | awk '/Loaded image/{print $NF}' | tail -1)"
got="$(docker run --rm "$ref")"
docker rmi -f "$ref" >/dev/null 2>&1 || true
[ "$got" = "built by ko" ] || { echo "FAIL: built image printed '$got'"; exit 1; }
echo "smoke test passed (uid $user, ko build ran the image's go toolchain)"
