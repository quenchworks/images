#!/usr/bin/env bash
# Smoke test for a built quench-rust image. Usage: test.sh <image-ref> <minor>
# where <minor> is the expected Rust minor version, e.g. 1.96.
#
# This is a LANGUAGE/SDK image: `cargo` IS the entrypoint, there is no
# long-running service to ping. So we exercise cargo + rustc directly under a
# READ-ONLY rootfs (writable /tmp for CARGO_HOME + target) and confirm nonroot.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> <minor>}"
MINOR="${2:?usage: test.sh <image-ref> <minor>}"   # e.g. 1.96

# The image is SHELL-LESS (no /bin/sh), so we can't script crate setup *inside*
# the container. Prepare the crate on the HOST and bind-mount it read-only.
# We create it UNDER the current dir (the checkout) rather than /tmp: a
# checkout-relative bind-mount is portable across native docker (CI) and Docker
# Desktop (dev), where /tmp may not be a shared path.
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT
mkdir -p "$WORK/src"
cat > "$WORK/Cargo.toml" <<EOF
[package]
name = "quench_hello"
version = "0.1.0"
edition = "2021"
[[bin]]
name = "quench_hello"
path = "src/main.rs"
EOF
cat > "$WORK/src/main.rs" <<'EOF'
fn main() { print!("quench-rust"); }
EOF

# The IMAGE ROOTFS is read-only (--read-only); writable toolchain state
# (CARGO_HOME + target, baked to /tmp in the image env) lands on a tmpfs.
# `:exec` lets cargo run the binary it builds. The user's PROJECT mount is
# read-write (it's the caller's own dir, not the image rootfs) -- cargo writes
# Cargo.lock there, exactly like a real build. CARGO_TARGET_DIR pushes compiled
# output onto the tmpfs so the project mount stays minimal.
RO=(--rm --read-only --tmpfs /tmp:exec
    -e CARGO_TARGET_DIR=/tmp/target
    -v "$WORK:/src" -w /src)

echo "== cargo --version matches $MINOR (default entrypoint) =="
cver="$(docker run "${RO[@]}" "$IMAGE" --version 2>&1)"
echo "$cver"
case "$cver" in
  "cargo $MINOR."*) : ;;
  *) echo "expected 'cargo $MINOR.*', got '$cver'"; exit 1 ;;
esac

echo "== rustc --version matches $MINOR =="
rver="$(docker run "${RO[@]}" --entrypoint /usr/bin/rustc "$IMAGE" --version 2>&1)"
echo "$rver"
case "$rver" in
  "rustc $MINOR."*) : ;;
  *) echo "expected 'rustc $MINOR.*', got '$rver'"; exit 1 ;;
esac

echo "== runs as nonroot uid 1001 (configured user) =="
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected configured user 1001, got '$user'"; exit 1; }

echo "== a real crate compiles AND runs (cargo run, read-only rootfs) =="
# The image's baked RUSTFLAGS make the gcc link step work on a hardened rootfs;
# CARGO_HOME + CARGO_TARGET_DIR live on the writable tmpfs. --offline forces a
# hermetic build (no crates.io fetch) -- our crate has zero deps, so it must
# succeed offline. The PROJECT mount is read-write (cargo writes Cargo.lock).
out="$(docker run "${RO[@]}" "$IMAGE" run --quiet --offline 2>/dev/null)"
[ "$out" = "quench-rust" ] || { echo "cargo run output wrong: '$out'"; exit 1; }

echo "== cargo build produces a binary, which then runs (read-only rootfs) =="
# `cargo build` does the gcc link (the bit our baked RUSTFLAGS fix), emitting the
# binary under CARGO_TARGET_DIR on the tmpfs; then execute it in the same
# container via `cargo run` (re-uses the cached build) to prove it's runnable.
out2="$(docker run "${RO[@]}" "$IMAGE" build --quiet --offline 2>/dev/null && \
        docker run "${RO[@]}" "$IMAGE" run --quiet --offline 2>/dev/null)"
[ "$out2" = "quench-rust" ] || { echo "cargo build+run output wrong: '$out2'"; exit 1; }

echo "== read-only rootfs is enforced (build artifact to read-only / must fail) =="
# Point CARGO_TARGET_DIR at a path on the read-only rootfs: the write must fail.
if docker run --rm --read-only --tmpfs /tmp:exec -e CARGO_HOME=/tmp/cargo \
     -e CARGO_TARGET_DIR=/nope -v "$WORK:/src" -w /src "$IMAGE" \
     build --quiet --offline >/dev/null 2>&1; then
  echo "rootfs was writable, expected read-only"; exit 1
fi

echo "smoke test passed (Rust $MINOR, configured user: $user, read-only rootfs)"
