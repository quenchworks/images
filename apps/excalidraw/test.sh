#!/usr/bin/env bash
# Smoke test for a built Excalidraw image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-excalidraw-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE"
# nginx serves the SPA on 8080; mount a writable tmpfs at /tmp for the read-only rootfs
docker run -d --name "$NAME" --tmpfs /tmp:rw,mode=1777 -p 127.0.0.1:8080:8080 "$IMAGE" >/dev/null

# index.html must be served (200) once nginx is up
for i in $(seq 1 30); do
  if curl -fsS http://127.0.0.1:8080/ >/dev/null 2>&1; then
    break
  fi
  [ "$i" = 30 ] && { echo "excalidraw did not start serving"; docker logs "$NAME"; exit 1; }
  sleep 1
done

# the served root must actually be the Excalidraw app
body="$(curl -fsS http://127.0.0.1:8080/)"
echo "$body" | grep -qi 'excalidraw' \
  || { echo "served page does not look like Excalidraw"; echo "$body" | head -20; exit 1; }

# a hashed JS asset must be served with a JavaScript content-type (MIME mapping works)
asset="$(printf '%s' "$body" | grep -oE '/assets/[^"]+\.js' | head -1 || true)"
if [ -n "$asset" ]; then
  ct="$(curl -fsSI "http://127.0.0.1:8080${asset}" | tr -d '\r' | awk -F': ' 'tolower($1)=="content-type"{print $2}')"
  echo "asset $asset -> $ct"
  case "$ct" in
    *javascript*|*ecmascript*) : ;;
    *) echo "JS asset served with wrong content-type: '$ct'"; exit 1 ;;
  esac
fi

# must run as the nonroot excalidraw user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
