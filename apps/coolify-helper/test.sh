#!/usr/bin/env bash
# Smoke test for a built Coolify helper image. Usage: test.sh <image-ref>
#
# coolify-helper is Coolify's on-demand BUILD ENGINE and is PRIVILEGED BY DESIGN: in
# production the Coolify control plane runs it as ROOT with the HOST docker socket mounted
# and `docker exec`s build commands into the long-lived container (CMD `tail -f /dev/null`).
# We can't mount a host daemon in CI, so this test proves the SURFACE is intact instead:
#   * every bundled build tool runs and reports its pinned version
#     (docker, docker compose, docker buildx, pack, railpack, nixpacks, mise, mc,
#      git, git-lfs, ssh);
#   * tini is PID 1 and the railpack<->mise seeding entrypoint actually seeds
#     /tmp/railpack/mise/mise-<version> from /usr/local/bin/mise on start;
#   * the image runs as ROOT (uid 0) -- asserted POSITIVELY, because that is the
#     by-design posture this tier requires to reach the host docker socket.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-coolify-helper-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

# Versions pinned in melange.yaml / tracked from Wolfi -- assert the bundled tools match.
MISE_VERSION="2026.3.17"
RAILPACK_VERSION="0.23.0"
NIXPACKS_VERSION="1.41.0"
# Must equal vars.pack-version in melange.yaml, which is what the build actually
# checks out and stamps. This said 0.38.2 (the version upstream bundles) long after
# the recipe moved to 0.40.9 for the lifecycle v0.21.19 line, so the boot test failed
# a correct image on both arches in run 35490170007.
PACK_VERSION="0.40.9"

echo "starting $IMAGE (long-lived build-engine container; runs as root by design)"
docker run -d --name "$NAME" "$IMAGE" >/dev/null

# Wait for the container to be up (CMD is `tail -f /dev/null`, so it should stay running).
for i in $(seq 1 30); do
  if docker ps --format '{{.Names}}' | grep -q "^${NAME}$"; then break; fi
  sleep 1
done
docker ps --format '{{.Names}}' | grep -q "^${NAME}$" \
  || { echo "container did not stay up"; docker logs "$NAME"; exit 1; }

dx() { docker exec "$NAME" "$@"; }

echo "== build toolchain versions =="
echo "- docker CLI:";       dx docker --version
echo "- docker compose:";   dx docker compose version
echo "- docker buildx:";    dx docker buildx version
echo "- pack:";             dx pack version
echo "- railpack:";         dx railpack --version
echo "- nixpacks:";         dx nixpacks --version
echo "- mise:";             dx mise --version
echo "- mc (minio):";       dx mc --version
echo "- git:";              dx git --version
echo "- git-lfs:";          dx git-lfs version
echo "- ssh:";              dx ssh -V

# Assert the from-source tools report the pinned versions (proves we built the right tag,
# not whatever happened to be on PATH).
dx mise --version      | grep -q "$MISE_VERSION"     || { echo "mise version mismatch (want $MISE_VERSION)"; exit 1; }
dx railpack --version  | grep -q "$RAILPACK_VERSION" || { echo "railpack version mismatch (want $RAILPACK_VERSION)"; exit 1; }
dx nixpacks --version  | grep -q "$NIXPACKS_VERSION" || { echo "nixpacks version mismatch (want $NIXPACKS_VERSION)"; exit 1; }
dx pack version        | grep -q "$PACK_VERSION"     || { echo "pack version mismatch (want $PACK_VERSION)"; exit 1; }

# The compose + buildx plugins must be wired as `docker compose` / `docker buildx`
# subcommands (Wolfi installs them under /usr/libexec/docker/cli-plugins).
dx docker compose version >/dev/null || { echo "docker compose plugin not wired"; exit 1; }
dx docker buildx version  >/dev/null || { echo "docker buildx plugin not wired"; exit 1; }

# tini must be PID 1 (it reaps the short-lived docker/build children the helper spawns).
pid1="$(dx cat /proc/1/cmdline | tr '\0' ' ')"
echo "PID 1 = $pid1"
echo "$pid1" | grep -q "tini" || { echo "expected tini as PID 1, got: $pid1"; exit 1; }

# The entrypoint must have seeded railpack's mise at the path railpack hardcodes,
# copied from our from-source /usr/local/bin/mise (so railpack never downloads a glibc mise).
dx test -x "/tmp/railpack/mise/mise-${MISE_VERSION}" \
  || { echo "entrypoint did not seed /tmp/railpack/mise/mise-${MISE_VERSION}"; exit 1; }
dx "/tmp/railpack/mise/mise-${MISE_VERSION}" --version | grep -q "$MISE_VERSION" \
  || { echo "seeded railpack mise does not run / wrong version"; exit 1; }
echo "railpack mise seed OK: /tmp/railpack/mise/mise-${MISE_VERSION}"

# PRIVILEGED-TIER ASSERTION: this tier MUST run as root (uid 0) to reach the host docker
# socket. We assert it positively -- a nonroot helper would be the bug here, not the fix.
uid="$(dx id -u)"
[ "$uid" = "0" ] || { echo "expected root (uid 0) for the privileged helper tier, got uid $uid"; exit 1; }
cfguser="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ -z "$cfguser" ] || [ "$cfguser" = "root" ] || [ "$cfguser" = "0" ] \
  || { echo "image Config.User should be root/empty for the privileged tier, got '$cfguser'"; exit 1; }

echo "smoke test passed (PRIVILEGED-BY-DESIGN root helper; docker+compose+buildx, pack/railpack/nixpacks/mise, mc, git/git-lfs/ssh all run; tini PID 1; railpack mise seeded)"
