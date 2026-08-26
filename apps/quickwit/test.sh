#!/usr/bin/env bash
# Smoke test for a built Quickwit image. Usage: test.sh <image-ref>
# Runs read-only rootfs + writable tmpfs for the data dir (the read-only posture the
# chart ships), waits for /health/livez, then exercises the REST API: liveness,
# readiness, the version endpoint, an index-list roundtrip, and the bundled admin UI
# at /ui. Also asserts the nonroot user and prints `quickwit --version`.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-quickwit-smoke-$$"

cleanup() {
  docker rm -f "$NAME" >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "starting $IMAGE (read-only rootfs, tmpfs /quickwit/qwdata + /tmp)"
# tmpfs the DATA DIR only. Mounting tmpfs on /quickwit shadows the whole directory,
# including the shipped /quickwit/config/quickwit.yaml, and the node dies with
# "failed to read file file:///quickwit/config/quickwit.yaml". The chart's emptyDir
# is likewise mounted at qwdata, not at /quickwit.
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /quickwit/qwdata:rw,mode=1777 \
  --tmpfs /tmp:rw,mode=1777 \
  -p 127.0.0.1:7280:7280 \
  -p 127.0.0.1:7281:7281 \
  "$IMAGE" >/dev/null

# wait for /health/livez (200 once the REST server is up).
for i in $(seq 1 90); do
  code="$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:7280/health/livez" 2>/dev/null || true)"
  if [ "$code" = "200" ]; then
    echo "/health/livez OK after ${i}s"
    break
  fi
  if ! docker ps --format '{{.Names}}' | grep -q "^${NAME}$"; then
    echo "container exited"; docker logs "$NAME"; exit 1
  fi
  [ "$i" = 90 ] && { echo "quickwit /health/livez did not become healthy"; docker logs "$NAME"; exit 1; }
  sleep 1
done

# readiness
echo "checking /health/readyz"
for i in $(seq 1 60); do
  code="$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:7280/health/readyz" 2>/dev/null || true)"
  [ "$code" = "200" ] && { echo "/health/readyz OK after ${i}s"; break; }
  [ "$i" = 60 ] && { echo "/health/readyz not 200 (got $code)"; docker logs "$NAME"; exit 1; }
  sleep 1
done

# version endpoint returns build info JSON
echo "checking /api/v1/version"
curl -fsS "http://127.0.0.1:7280/api/v1/version" | grep -q '"build"' \
  || { echo "version endpoint JSON missing"; docker logs "$NAME"; exit 1; }

# index list (real REST roundtrip; empty node returns a JSON array)
echo "checking /api/v1/indexes"
curl -fsS "http://127.0.0.1:7280/api/v1/indexes" | grep -q '\[' \
  || { echo "/api/v1/indexes failed"; docker logs "$NAME"; exit 1; }
echo "index list OK"

# bundled admin UI at /ui (200 with index.html served)
echo "checking /ui"
code="$(curl -s -o /dev/null -w '%{http_code}' -L "http://127.0.0.1:7280/ui/search" 2>/dev/null || true)"
[ "$code" = "200" ] || { echo "/ui not 200 (got $code)"; docker logs "$NAME"; exit 1; }
echo "/ui OK (admin UI bundled)"

# must run as the nonroot quickwit user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "version:"; docker exec "$NAME" /usr/bin/quickwit --version
echo "smoke test passed (nonroot user: $user, read-only rootfs, /health livez+readyz, version, index list, /ui bundled)"
