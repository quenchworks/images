#!/usr/bin/env bash
# Smoke test for a built OpenSearch image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-opensearch-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE"
# The remote-reindex assertion below needs its target host allowlisted. Two traps here,
# both of which used to make that assertion fail 400 before any HTTP client was involved:
#   * the setting is reindex.remote.ALLOWlist on OpenSearch 3.x (it was whitelist), and
#   * OpenSearch only reads node settings from opensearch.yml / -E, NOT from -D JVM
#     properties -- so passing it in OPENSEARCH_JAVA_OPTS silently did nothing (and
#     clobbered the image's own -Xms/-Xmx while it was at it).
# OPENSEARCH_CONFIG_EXTRA is the entrypoint's documented escape hatch: raw opensearch.yml.
docker run -d --name "$NAME" -p 127.0.0.1:9200:9200 \
  -e 'OPENSEARCH_CONFIG_EXTRA=reindex.remote.allowlist: ["127.0.0.1:9200", "localhost:9200"]' \
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

# --- notifications webhook: the ONE path that exercises the SHADED httpclient5 -------
# The reindex above uses the STANDALONE httpclient5 jar. melange block (2d) additionally
# relocates fixed httpcore5/httpclient5 bytecode into opensearch-notifications-core-spi,
# where upstream shades its own copy -- a different set of class files that nothing above
# touches. A webhook notification is what drives that copy, so send one and require the
# sink's own HTTP answer to come back: a bad relocation cannot produce an HTTP reason
# phrase, it produces NoClassDefFoundError/VerifyError instead. This is the assertion
# that distinguishes "the shaded code was really replaced" from "the scanner went quiet".
echo "checking a webhook notification (exercises the SHADED httpclient5 in notifications-core)"
curl -fsS -XPOST 'http://127.0.0.1:9200/_plugins/_notifications/configs' \
  -H 'Content-Type: application/json' -d '{
    "config_id": "smoke-webhook",
    "config": { "name": "smoke", "description": "smoke test webhook",
                "config_type": "webhook", "is_enabled": true,
                "webhook": { "url": "http://localhost:9200/hooksink/_doc/1" } } }' >/dev/null
# The canned test message is plain text, so the sink (this node) answers 400 Bad Request.
# That reason phrase travelling back through the shaded client IS the assertion.
wh="$(curl -sS -XPOST 'http://127.0.0.1:9200/_plugins/_notifications/feature/test/smoke-webhook' 2>&1 || true)"
case "$wh" in
  *NoClassDefFoundError*|*ClassNotFoundException*|*VerifyError*|*NoSuchMethodError*)
    echo "the shaded httpclient5 failed to link -- the relocation in melange block (2d) is broken"
    echo "$wh" | head -5; docker logs "$NAME" 2>&1 | tail -30; exit 1 ;;
  *'Failed: Bad Request'*)
    echo "  the sink's HTTP answer came back through the shaded client" ;;
  *)
    echo "webhook delivery gave an unexpected result -- check the shaded httpclient5 first"
    echo "$wh" | head -5; docker logs "$NAME" 2>&1 | tail -30; exit 1 ;;
esac

echo "smoke test passed (nonroot user: $user)"
