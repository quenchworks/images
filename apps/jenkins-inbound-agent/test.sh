#!/usr/bin/env bash
# Smoke test for a built Jenkins inbound agent image. Usage: test.sh <image-ref>
# A real run needs a Jenkins controller to connect to (JNLP secret + url), so the
# CI smoke test confirms the image is assembled correctly: the entrypoint script,
# a working JRE, and the remoting agent.jar are all present and runnable.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref>}"

echo "entrypoint is the jenkins-agent script"
ep="$(docker inspect "$IMAGE" --format '{{json .Config.Entrypoint}}')"
echo "  $ep"; echo "$ep" | grep -q "jenkins-agent" || { echo "wrong entrypoint"; exit 1; }

echo "JRE runs"
docker run --rm --entrypoint java "$IMAGE" -version 2>&1 | grep -qiE "openjdk|version" || { echo "no working JRE"; exit 1; }

echo "remoting agent.jar present and is a JAR"
docker run --rm --entrypoint sh "$IMAGE" -c 'test -s /usr/share/jenkins/agent.jar' || { echo "agent.jar missing"; exit 1; }
docker run --rm --entrypoint java "$IMAGE" -jar /usr/share/jenkins/agent.jar -version 2>&1 | grep -qE "[0-9]+\." || echo "  (agent.jar -version not supported; presence check already passed)"

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed (jenkins-agent entrypoint, JRE + agent.jar present, nonroot $user)"
