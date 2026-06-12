#!/usr/bin/env bash
# Smoke test for a built OpenSearch image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-opensearch-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE"
docker run -d --name "$NAME" -p 127.0.0.1:9200:9200 "$IMAGE" >/dev/null

# OpenSearch + JVM start takes a while; poll the HTTP API for a green/yellow status
ok=""
for i in $(seq 1 60); do
  if curl -fsS http://127.0.0.1:9200/ 2>/dev/null | grep -q '"cluster_name"'; then
    ok=1; break
  fi
  [ "$i" = 60 ] && { echo "opensearch did not become ready"; docker logs "$NAME" | tail -40; exit 1; }
  sleep 3
done
echo "API up:"; curl -fsS http://127.0.0.1:9200/ 2>/dev/null | grep -E '"number"|"cluster_name"' | head -2

echo "checking cluster health"
curl -fsS 'http://127.0.0.1:9200/_cluster/health' 2>/dev/null | grep -qE '"status":"(green|yellow)"' \
  || { echo "cluster not healthy"; exit 1; }

# index a doc and read it back
curl -fsS -XPOST 'http://127.0.0.1:9200/smoke/_doc/1?refresh=true' -H 'Content-Type: application/json' -d '{"k":"quench"}' >/dev/null
curl -fsS 'http://127.0.0.1:9200/smoke/_doc/1' 2>/dev/null | grep -q '"quench"' || { echo "index/get failed"; exit 1; }

# must run as the nonroot opensearch user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
