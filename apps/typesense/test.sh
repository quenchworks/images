#!/usr/bin/env bash
# Smoke test for a built Typesense image. Usage: test.sh <image-ref>
# Runs with a READ-ONLY rootfs and a writable tmpfs /data mount, waits for GET
# /health to return {"ok":true}, then exercises an authenticated collection
# create + document index + search roundtrip over the REST API (8108). Confirms
# nonroot uid 1001 and that auth is enforced.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-typesense-smoke-$$"
API_KEY="smoke-key-123"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE (read-only rootfs, writable tmpfs /data)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /data:rw,mode=1777,exec \
  --tmpfs /tmp:rw,mode=1777 \
  -p 18108:8108 \
  -e TYPESENSE_API_KEY="$API_KEY" \
  "$IMAGE" >/dev/null

# confirm nonroot uid 1001
UID_OUT="$(docker exec "$NAME" id -u)"
echo "runtime uid: $UID_OUT"
[ "$UID_OUT" = "1001" ] || { echo "FAIL: not running as uid 1001"; exit 1; }

# wait for GET /health -> {"ok":true}
echo "waiting for /health"
ok=""
for i in $(seq 1 60); do
  if curl -fsS "http://localhost:18108/health" 2>/dev/null | grep -q '"ok":true'; then
    ok=1; echo "health ok after ${i}s"; break
  fi
  if [ "$i" = 60 ]; then
    echo "FAIL: typesense did not become healthy"; docker logs "$NAME" 2>&1 | tail -60; exit 1
  fi
  sleep 1
done

h() { curl -fsS -H "X-TYPESENSE-API-KEY: $API_KEY" "$@"; }

echo "create collection:"
h -X POST "http://localhost:18108/collections" \
  -H 'Content-Type: application/json' \
  -d '{"name":"books","fields":[{"name":"title","type":"string"}]}' >/dev/null
echo "  created"

echo "index a document:"
h -X POST "http://localhost:18108/collections/books/documents" \
  -H 'Content-Type: application/json' \
  -d '{"id":"1","title":"The Hitchhiker Guide"}' >/dev/null
echo "  indexed"

echo "search (typo-tolerant):"
HITS="$(h "http://localhost:18108/collections/books/documents/search?q=hitchiker&query_by=title" | grep -o '"found":[0-9]*' | head -1)"
echo "  -> $HITS"
[ "$HITS" = '"found":1' ] || { echo "FAIL: expected 1 hit, got '$HITS'"; exit 1; }

# verify auth is enforced (missing key must be rejected)
echo "auth enforcement check (no api key must fail):"
if curl -fsS "http://localhost:18108/collections" >/dev/null 2>&1; then
  echo "FAIL: request succeeded without api key"; exit 1
fi
echo "  unauthenticated request correctly rejected"

echo "PASS: typesense smoke test green"
