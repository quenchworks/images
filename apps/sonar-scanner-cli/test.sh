#!/usr/bin/env bash
# Smoke test for a built SonarScanner CLI image. Usage: test.sh <image-ref>
# A real scan needs a SonarQube server + a project, so the CI smoke test just confirms
# the image is assembled: the sonar-scanner-cli binary on PATH and a working bundled
# JRE (the Wolfi openjdk apk installs java under $JAVA_HOME/bin, not /usr/bin), run
# inside the image's own environment (which presets JAVA_HOME), plus the nonroot user.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref>}"

docker run --rm --entrypoint /bin/sh "$IMAGE" -c '
  set -e
  command -v sonar-scanner-cli >/dev/null || { echo "sonar-scanner-cli not on PATH"; exit 1; }
  java="$(command -v java || true)"; [ -n "$java" ] || java="$JAVA_HOME/bin/java"
  "$java" -version 2>&1 | grep -qiE "openjdk|version" || { echo "no working JRE (JAVA_HOME=$JAVA_HOME)"; exit 1; }
' || { echo "smoke failed"; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed (sonar-scanner-cli + bundled JRE present, nonroot $user)"
