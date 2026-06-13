#!/usr/bin/env bash
# Smoke test for a built Qdrant image. Usage: test.sh <image-ref>
# Runs read-only rootfs + writable tmpfs for the storage dir (the read-only posture
# the chart ships), waits for /healthz, then exercises the REST API: root version
# JSON, /readyz, /livez, collections list, a create-collection + list roundtrip, and
# the bundled web dashboard at /dashboard.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-qdrant-smoke-$$"

cleanup() {
  docker rm -f "$NAME" >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "starting $IMAGE (read-only rootfs, tmpfs /qdrant + /tmp)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /qdrant:rw,mode=1777 \
  --tmpfs /tmp:rw,mode=1777 \
  -p 127.0.0.1:6333:6333 \
  -p 127.0.0.1:6334:6334 \
  "$IMAGE" >/dev/null

# wait for /healthz (200 once the HTTP server is up).
for i in $(seq 1 60); do
  code="$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:6333/healthz" 2>/dev/null || true)"
  if [ "$code" = "200" ]; then
    echo "/healthz OK after ${i}s"
    break
  fi
  if ! docker ps --format '{{.Names}}' | grep -q "^${NAME}$"; then
    echo "container exited"; docker logs "$NAME"; exit 1
  fi
  [ "$i" = 60 ] && { echo "qdrant /healthz did not become healthy"; docker logs "$NAME"; exit 1; }
  sleep 1
done

# root returns qdrant version JSON
echo "checking / (version JSON), /livez, /readyz"
curl -fsS "http://127.0.0.1:6333/" | grep -q '"version"' || { echo "root version JSON missing"; docker logs "$NAME"; exit 1; }
[ "$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:6333/livez)" = "200" ] || { echo "/livez not 200"; exit 1; }
[ "$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:6333/readyz)" = "200" ] || { echo "/readyz not 200"; exit 1; }

# collections list
echo "checking /collections"
curl -fsS "http://127.0.0.1:6333/collections" | grep -q '"result"' || { echo "/collections failed"; docker logs "$NAME"; exit 1; }

# create a collection + verify it lists (real API roundtrip)
echo "create-collection roundtrip"
curl -fsS -X PUT "http://127.0.0.1:6333/collections/gate" \
  -H 'content-type: application/json' \
  -d '{"vectors":{"size":4,"distance":"Dot"}}' | grep -q '"result":true' \
  || { echo "create collection failed"; docker logs "$NAME"; exit 1; }
curl -fsS "http://127.0.0.1:6333/collections" | grep -q '"gate"' \
  || { echo "created collection not listed"; docker logs "$NAME"; exit 1; }
echo "collection roundtrip OK"

# bundled web dashboard at /dashboard (200 with index.html served)
echo "checking /dashboard"
code="$(curl -s -o /dev/null -w '%{http_code}' -L "http://127.0.0.1:6333/dashboard/" 2>/dev/null || true)"
[ "$code" = "200" ] || { echo "/dashboard not 200 (got $code)"; docker logs "$NAME"; exit 1; }
echo "/dashboard OK"

# must run as the nonroot qdrant user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "version:"; docker exec "$NAME" /usr/bin/qdrant --version
echo "smoke test passed (nonroot user: $user, read-only rootfs, /healthz /livez /readyz, collection roundtrip, /dashboard bundled)"
