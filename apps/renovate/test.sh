#!/usr/bin/env bash
# Smoke test for a built Renovate image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

echo "renovate --version:"
out="$(docker run --rm "$IMAGE" --version 2>&1)"
echo "$out"
# version must print a real semver (proves node runtime + bundle resolve)
echo "$out" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "renovate --version did not print a semver"; exit 1; }

# must run as the nonroot renovate user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

# Functional check: extract dependencies from a fixture with the local platform. With no git
# repo renovate lists files through glob, and each manager matches them through minimatch, so
# this exercises the swapped glob 14 / minimatch 11 (melange.yaml), which --version never loads.
# The fixture sits under $PWD: Docker Desktop cannot mount /tmp.
fix="$(mktemp -d "${PWD}/.renovate-fixture.XXXXXX")"
trap 'rm -rf "$fix"' EXIT
mkdir -p "$fix/sub"
printf '{"name":"x","dependencies":{"left-pad":"1.0.0"}}\n' > "$fix/package.json"
printf 'FROM node:20.1.0\n' > "$fix/sub/Dockerfile"
printf '{"extends":[],"enabledManagers":["npm","dockerfile"]}\n' > "$fix/renovate.json"
chmod -R a+rX "$fix"
log="$(docker run --rm -v "$fix:/repo:ro" -w /repo -e LOG_LEVEL=debug -e RENOVATE_PLATFORM=local \
  -e RENOVATE_DRY_RUN=extract -e HOME=/tmp -e RENOVATE_BASE_DIR=/tmp/renovate "$IMAGE" 2>&1)" || true
for want in '"depName": "left-pad"' '"depName": "node"' 'using glob instead'; do
  grep -qF "$want" <<<"$log" || { echo "renovate extract missing: $want"; echo "$log" | tail -40; exit 1; }
done
echo "extract check passed (left-pad + node found via glob and minimatch)"

echo "smoke test passed (nonroot user: $user)"
