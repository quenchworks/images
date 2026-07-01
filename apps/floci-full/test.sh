#!/usr/bin/env bash
# Smoke test for a built floci-full image. Usage: test.sh <image-ref>
# This is the NON-hardened FULL variant: it runs as root and, in production,
# talks to a mounted host Docker socket (socket API, no docker CLI needed).
# The gate has NO Docker socket / DinD, so we do NOT start any Docker-backed
# service (Lambda, RDS, ECS, ...). We only verify:
#   1. the image runs as root (uid 0),
#   2. the server boots and GET /_floci/health returns HTTP 200 with a JSON body
#      that includes the "services" map.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-floci-full-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

# must run as root (uid 0) -- this is the deliberately non-hardened variant
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
case "$user" in
  ""|"0"|"root") echo "root user confirmed: '${user:-<empty=root>}'" ;;
  *) echo "expected root (uid 0), got '$user'"; exit 1 ;;
esac

echo "version (java -version on the runtime JRE):"
docker run --rm --entrypoint /usr/lib/jvm/java-25-openjdk/bin/java "$IMAGE" -version

echo "starting $IMAGE (no docker.sock mounted -- Docker-backed services stay idle)"
docker run -d --name "$NAME" -p 127.0.0.1:0:4566 "$IMAGE" >/dev/null
# resolve the host port docker mapped to the container's 4566 edge port
PORT="$(docker port "$NAME" 4566/tcp | head -1 | sed 's/.*://')"
[ -n "$PORT" ] || { echo "could not resolve mapped port"; docker logs "$NAME" | tail -50; exit 1; }
echo "container :4566 mapped to host :$PORT"
BASE="http://127.0.0.1:${PORT}"

# Floci boots the JVM in ~5-30s; poll /_floci/health for HTTP 200 up to ~120s.
ready=0; code=""
for i in $(seq 1 60); do
  code="$(curl -s -o /dev/null -w '%{http_code}' "${BASE}/_floci/health" || true)"
  if [ "$code" = "200" ]; then
    echo "floci health up after ~$((i*2))s (HTTP $code)"; ready=1; break
  fi
  sleep 2
done

if [ "$ready" != 1 ]; then
  echo "floci health endpoint did not respond in time (last HTTP '$code')"
  docker logs "$NAME" | tail -60; exit 1
fi

echo "health payload:"
body="$(curl -s "${BASE}/_floci/health")"
echo "$body" | head -c 400; echo

# The health JSON must carry the services map ({"services":{...}}).
echo "$body" | grep -q '"services"' || { echo "health body missing \"services\" map"; docker logs "$NAME" | tail -40; exit 1; }

echo "smoke test passed (root user, health HTTP $code with services map)"
