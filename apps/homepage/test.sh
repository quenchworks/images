#!/usr/bin/env bash
# Smoke test for a built Homepage image. Usage: test.sh <image-ref> [version]
#
# Runs the image the way the chart does: uid 1001, READ-ONLY rootfs, a writable
# /app/config holding one services.yaml. Then:
#   1. /api/healthcheck answers "up".
#   2. /api/services returns the test group and service from services.yaml:
#      the app read its config dir.
#   3. A request with an unlisted Host header gets 400 from the host check.
#   4. The page renders, and the configs the server reads (settings.yaml,
#      docker.yaml) were copied in from the skeleton; without the skeleton the
#      app exits.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
NAME="quench-homepage-smoke-$$"
PORT=13000
# Under $HOME, not /tmp: Docker Desktop cannot bind-mount /tmp paths.
work="$(mktemp -d "$HOME/.quench-homepage-test.XXXXXX")"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$work"; }
trap cleanup EXIT

cat > "$work/services.yaml" <<'YML'
- Quench Group:
    - Quench Service:
        href: http://quench.invalid/
        description: smoke test entry
YML
chmod -R a+rwX "$work"

# The host check allows localhost:<PORT> by default, and PORT is also the listen
# port, so the container listens on the host's test port too.
docker run -d --name "$NAME" --read-only --tmpfs /tmp:rw,mode=1777 \
  -e PORT="$PORT" -v "$work:/app/config" -p "127.0.0.1:${PORT}:${PORT}" "$IMAGE" >/dev/null

h=""
for i in $(seq 1 60); do
  h="$(curl -fsS -m 5 "http://localhost:${PORT}/api/healthcheck" 2>/dev/null || true)"
  [ "$h" = up ] && break
  docker ps --filter "name=$NAME" --filter status=running --format '{{.Names}}' | grep -q "$NAME" \
    || { echo "homepage died during startup:"; docker logs "$NAME"; exit 1; }
  [ "$i" = 60 ] && { echo "healthcheck never answered up"; docker logs "$NAME"; exit 1; }
  sleep 1
done
echo "healthcheck: $h"

svc="$(curl -fsS -m 10 "http://localhost:${PORT}/api/services")"
echo "$svc" | head -c 300; echo
grep -q 'Quench Group' <<<"$svc" && grep -q 'Quench Service' <<<"$svc" || { echo "services.yaml was not read"; exit 1; }

code="$(curl -s -o /dev/null -w '%{http_code}' -m 5 -H 'Host: evil.example' "http://127.0.0.1:${PORT}/")"
[ "$code" = 400 ] || { echo "unlisted Host got $code, expected 400"; exit 1; }
echo "unlisted Host refused (400)"

page="$(curl -fsS -m 20 "http://localhost:${PORT}/")"
grep -qi '<html' <<<"$page" || { echo "/ did not render HTML"; exit 1; }
# The app copies a skeleton file in lazily, when something first needs it; the
# server reads these two at startup and on the first render (widgets.yaml and
# bookmarks.yaml only when the browser asks for them).
for f in settings.yaml docker.yaml; do
  [ -f "$work/$f" ] || { echo "$f was not copied from the skeleton"; docker logs "$NAME"; exit 1; }
done
echo "skeleton configs copied into the config dir"
if [ -n "${2:-}" ]; then
  docker exec "$NAME" /usr/bin/node -e "process.stdout.write(require('/usr/share/homepage/package.json').version)" | grep -qx "$2" \
    || { echo "package version is not $2"; exit 1; }
fi

if docker logs "$NAME" 2>&1 | grep -qE 'Read-only file system|EROFS|EACCES|Failed to initialize'; then
  echo "found write errors in logs:"; docker logs "$NAME"; exit 1
fi
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed (nonroot $user, read-only rootfs, config read, host check live)"
