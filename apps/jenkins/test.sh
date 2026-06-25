#!/usr/bin/env bash
# Smoke test for a built jenkins image. Usage: test.sh <image-ref>
# Boots Jenkins (slow JVM + setup-wizard init), then waits for the web UI on
# :8080 to answer. Jenkins serves /login with 200 or 403 once up (the setup
# wizard page is fine). Also prints `java -jar jenkins.war --version`.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-jenkins-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

# must run as the nonroot jenkins user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "nonroot user confirmed: $user"

echo "version:"
docker run --rm --entrypoint /usr/lib/jvm/java-21-openjdk/bin/java "$IMAGE" \
  -jar /usr/share/jenkins/jenkins.war --version

echo "starting $IMAGE"
docker run -d --name "$NAME" -p 127.0.0.1:0:8080 "$IMAGE" >/dev/null
# resolve the host port docker mapped to the container's 8080
PORT="$(docker port "$NAME" 8080/tcp | head -1 | sed 's/.*://')"
[ -n "$PORT" ] || { echo "could not resolve mapped port"; docker logs "$NAME" | tail -50; exit 1; }
echo "container :8080 mapped to host :$PORT"

# Jenkins boots in ~30-90s; poll the login page (HTTP status only) up to ~150s.
ready=0; code=""
for i in $(seq 1 75); do
  code="$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:${PORT}/login" || true)"
  case "$code" in
    200|403)
      echo "jenkins login page up after ~$((i*2))s (HTTP $code)"; ready=1; break ;;
  esac
  sleep 2
done

if [ "$ready" != 1 ]; then
  echo "jenkins did not serve the login page in time"; docker logs "$NAME" | tail -50; exit 1
fi

echo "smoke test passed (nonroot user: $user, login HTTP $code)"
