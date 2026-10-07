#!/usr/bin/env bash
# Smoke test for a built tomee image. Usage: test.sh <image-ref>
# Boots TomEE, waits for HTTP on 8080 (404 from the emptied webapps/), and checks
# that OpenEJB started (the Jakarta EE container, the part TomEE adds to Tomcat).
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-tomee-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

docker run -d --name "$NAME" -p 8080 "$IMAGE" >/dev/null
HOSTPORT="$(docker port "$NAME" 8080/tcp | head -1 | sed 's/.*://')"
[ -n "$HOSTPORT" ] || { echo "could not resolve published port"; docker logs "$NAME" 2>&1 | tail -40; exit 1; }
URL="http://localhost:${HOSTPORT}/"

code=""
for i in $(seq 1 90); do
  code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "$URL" || true)"
  case "$code" in 200|404) echo "HTTP $code after ~${i}s"; break ;; esac
  [ "$i" = 90 ] && { echo "tomee did not answer on $URL (last: $code)"; docker logs "$NAME" 2>&1 | tail -80; exit 1; }
  sleep 1
done

logs="$(docker logs "$NAME" 2>&1)"
grep -q 'org.apache.openejb' <<<"$logs" || { echo "no OpenEJB startup in the log"; tail -60 <<<"$logs"; exit 1; }
grep -q 'Server startup in' <<<"$logs" || { echo "no 'Server startup in' line"; tail -60 <<<"$logs"; exit 1; }
if grep -q 'SEVERE' <<<"$logs"; then
  echo "startup logged errors:"; grep -A3 'SEVERE' <<<"$logs" | head -40; exit 1
fi

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (HTTP $code, OpenEJB up, user $user)"
