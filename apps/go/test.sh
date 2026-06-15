#!/usr/bin/env bash
# Smoke test for a built quench-go image. Usage: test.sh <image-ref> <minor>
# where <minor> is the expected Go minor version, e.g. 1.26.
#
# This is a LANGUAGE/SDK image: the `go` driver IS the entrypoint, and the image
# is SHELL-LESS (no /bin/sh) -- so we can't script multi-file setup *inside* the
# container. Instead we prepare a tiny module on the HOST, bind-mount it
# read-only, and drive the `go` entrypoint directly. All writable toolchain
# state (GOCACHE/GOPATH/HOME, baked to /tmp in the image) lands on a tmpfs, so
# the rootfs stays read-only and the source mount stays read-only too.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> <minor>}"
MINOR="${2:?usage: test.sh <image-ref> <minor>}"   # e.g. 1.26

# Create the scratch module UNDER the current dir (the checkout) rather than
# /tmp: bind-mounting a checkout-relative path is portable across native docker
# (CI) and Docker Desktop (dev), where /tmp may not be a shared path.
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT
cat > "$WORK/go.mod" <<EOF
module quench/hello

go $MINOR
EOF
cat > "$WORK/main.go" <<'EOF'
package main

import "fmt"

func main() { fmt.Print("quench-go") }
EOF

# Read-only rootfs + read-only source mount; only /tmp (caches + build output)
# is writable, via tmpfs. `:exec` lets us run a binary we build into /tmp.
RO=(--rm --read-only --tmpfs /tmp:exec -v "$WORK:/src:ro" -w /src)

echo "== go version matches $MINOR (default entrypoint) =="
ver="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" version 2>&1)"
echo "$ver"
case "$ver" in
  "go version go$MINOR."*) : ;;
  *) echo "expected 'go version go$MINOR.*', got '$ver'"; exit 1 ;;
esac

echo "== configured to run as nonroot uid 1001 =="
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected configured user 1001, got '$user'"; exit 1; }

echo "== a real program compiles AND runs (go run, read-only rootfs + ro source) =="
out="$(docker run "${RO[@]}" "$IMAGE" run .)"
[ "$out" = "quench-go" ] || { echo "go run output wrong: '$out'"; exit 1; }

echo "== go build produces a working binary on a read-only rootfs =="
# Build into the writable tmpfs, then execute that artifact (entrypoint=go, so
# point it at the built binary via --entrypoint) to prove it actually runs.
docker run "${RO[@]}" "$IMAGE" build -o /tmp/hello .
out2="$(docker run "${RO[@]}" --entrypoint /usr/bin/go "$IMAGE" run .)"
[ "$out2" = "quench-go" ] || { echo "rebuild output wrong: '$out2'"; exit 1; }

echo "== read-only rootfs is enforced (build output to / must fail) =="
if docker run "${RO[@]}" "$IMAGE" build -o /should-fail . >/dev/null 2>&1; then
  echo "rootfs was writable, expected read-only"; exit 1
fi

echo "smoke test passed (Go $MINOR, configured user: $user, read-only rootfs)"
