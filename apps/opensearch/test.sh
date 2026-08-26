#!/usr/bin/env bash
# Smoke test for a built OpenSearch image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-opensearch-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE"
# reindex.remote.whitelist is set so the remote-reindex assertion below can run. That
# assertion is the only thing here that goes through httpclient5 (see melange block 2c);
# everything else uses the transport layer.
docker run -d --name "$NAME" -p 127.0.0.1:9200:9200 \
  -e "OPENSEARCH_JAVA_OPTS=-Dreindex.remote.whitelist=127.0.0.1:9200,localhost:9200" \
  "$IMAGE" >/dev/null

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

# The whole point of the calcite jar surgery (see melange.yaml) is that the SQL
# planner keeps working after we swap calcite-core. Nothing here proved that until
# now -- build.conf CLAIMED "_plugins/_sql returns 200" but that was a one-off manual
# check, so the recipe's own justification had no automated backing. A SELECT drives
# calcite's planner, which is exactly what a bad jar swap breaks (and it would still
# boot clean and pass every check above).
echo "checking the SQL plugin (exercises the patched calcite planner)"
sql="$(curl -fsS -XPOST 'http://127.0.0.1:9200/_plugins/_sql' \
  -H 'Content-Type: application/json' \
  -d '{"query": "SELECT k FROM smoke"}' 2>/dev/null || true)"
case "$sql" in
  *quench*) echo "  _plugins/_sql SELECT returned the indexed value" ;;
  *) echo "SQL query failed -- the calcite planner path is broken."; echo "response: $sql"; exit 1 ;;
esac

# must run as the nonroot opensearch user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

# --- remote reindex: the ONE path that exercises httpclient5 -----------------------
# Why this exists: the httpcomponents5 bump in melange block 2c is the same library move
# that broke graylog with "ZipException: Not in GZIP format" inside the OpenSearch client's
# JSON parser. index/get, cluster health and _plugins/_sql all travel the transport layer
# and would not notice. A reindex with a `remote` source goes through the REST client and
# httpclient5 even when the host is this same node, so it covers the gzip path for real.
echo "checking remote reindex (exercises the REST client + httpclient5)"
rr="$(curl -fsS -XPOST 'http://127.0.0.1:9200/_reindex?refresh=true' \
        -H 'Content-Type: application/json' -d '{
          "source": { "remote": { "host": "http://127.0.0.1:9200" }, "index": "smoke" },
          "dest":   { "index": "smoke-remote" } }' 2>&1 || true)"
case "$rr" in
  *'"failures":[]'*) echo "  remote reindex OK (httpclient5 path healthy)" ;;
  *) echo "remote reindex FAILED -- httpclient5/gzip regression is the first suspect"
     echo "$rr" | head -20
     docker logs "$NAME" 2>&1 | tail -30
     exit 1 ;;
esac
# and the copied document must actually be readable
curl -fsS 'http://127.0.0.1:9200/smoke-remote/_doc/1' 2>/dev/null | grep -q '"quench"' \
  || { echo "remote-reindexed doc did not round-trip"; exit 1; }
echo "  reindexed document round-tripped"

echo "smoke test passed (nonroot user: $user)"
