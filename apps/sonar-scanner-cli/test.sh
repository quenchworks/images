#!/usr/bin/env bash
# Smoke test for a built SonarScanner CLI image. Usage: test.sh <image-ref>
# A real scan needs a SonarQube server + a project, so the CI smoke test just
# confirms the scanner launches its JVM and reports a stamped version.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref>}"

echo "sonar-scanner version"
out="$(docker run --rm "$IMAGE" -v 2>&1 || docker run --rm "$IMAGE" --version 2>&1)"
echo "$out" | grep -qiE "SonarScanner|Scanner CLI" || { echo "no version banner"; echo "$out"; exit 1; }
ver="$(echo "$out" | grep -oiE "[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+" | head -1)"
echo "  reported: ${ver:-?}"
[ -n "$ver" ] || { echo "version not reported"; echo "$out"; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed (SonarScanner $ver, nonroot $user)"
