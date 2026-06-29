#!/usr/bin/env bash
# Smoke test for a built SonarScanner CLI image. Usage: test.sh <image-ref>
# A real scan needs a SonarQube server + a project (and `-v` makes the scanner attempt
# a run), so the CI smoke test just confirms the image is assembled: a working bundled
# JRE, the sonar-scanner-cli binary on PATH, and the nonroot user.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref>}"

echo "bundled JRE runs"
docker run --rm --entrypoint java "$IMAGE" -version 2>&1 | grep -qiE "openjdk|version" \
  || { echo "no working JRE"; exit 1; }

echo "sonar-scanner-cli is on PATH"
docker run --rm --entrypoint /bin/sh "$IMAGE" -c 'command -v sonar-scanner-cli >/dev/null' \
  || { echo "sonar-scanner-cli not found"; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed (JRE + sonar-scanner-cli present, nonroot $user)"
