#!/usr/bin/env bash
# Smoke test for a built tomcat image. Usage: test.sh <image-ref>
# Boots Tomcat, hits http://localhost:8080/ and asserts the server answers with a
# real HTTP status (200 or the expected 404 from the emptied ROOT) and that the
# response identifies as Tomcat/Coyote.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-tomcat-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE"
# explicitly publish container 8080 to an ephemeral host port (the minimal apko
# image carries no EXPOSE metadata, so -P alone would publish nothing).
docker run -d --name "$NAME" -p 8080 "$IMAGE" >/dev/null
HOSTPORT="$(docker port "$NAME" 8080/tcp | head -1 | sed 's/.*://')"
[ -n "$HOSTPORT" ] || { echo "could not resolve published port"; docker logs "$NAME" | tail -40; exit 1; }
URL="http://localhost:${HOSTPORT}/"

# Tomcat needs a few seconds to bind the connector; poll until it answers HTTP.
resp=""; code=""
for i in $(seq 1 60); do
  resp="$(curl -sS -i --max-time 5 "$URL" 2>/dev/null || true)"
  code="$(printf '%s\n' "$resp" | sed -n '1s#.* \([0-9][0-9][0-9]\) .*#\1#p' | head -1)"
  if [ -n "$code" ]; then echo "server answered HTTP $code after ~$((i))s"; break; fi
  [ "$i" = 60 ] && { echo "tomcat did not answer on $URL"; docker logs "$NAME" | tail -60; exit 1; }
  sleep 1
done

# empty ROOT => 404 is the expected/acceptable response; 200 also fine.
case "$code" in
  200|404) : ;;
  *) echo "unexpected HTTP status: $code"; printf '%s\n' "$resp" | head -20; exit 1 ;;
esac

# identity check: either the Coyote 'Server:' header or the Tomcat error page body
if printf '%s\n' "$resp" | grep -qiE 'Server:.*(Coyote|Tomcat)' \
   || printf '%s\n' "$resp" | grep -qi 'Apache Tomcat'; then
  echo "identified as Tomcat/Coyote"
else
  echo "response did not identify as Tomcat"; printf '%s\n' "$resp" | head -30; exit 1
fi

# must run as the nonroot tomcat user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (HTTP $code, nonroot user: $user)"
