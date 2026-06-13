#!/usr/bin/env bash
# Smoke test for a built Elasticsearch image. Usage: test.sh <image-ref>
#
# !!! LICENSE: Elasticsearch's default distribution is dual SSPL-1.0 / Elastic License
# !!! 2.0 -- NEITHER is OSI-approved open source. CAUTION TIER. The truly-open drop-in
# !!! alternative is OpenSearch (Apache-2.0, shipped at catalog #11) -- prefer it.
# Security is DISABLED by default (matches the QuenchWorks single-node posture), so the
# smoke test hits the HTTP API unauthenticated.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-elasticsearch-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE (read-only rootfs + writable tmpfs mounts)"
# Hardened posture: read-only rootfs with writable tmpfs for conf/data/logs + /tmp.
# /tmp MUST be writable: ES 9.x's entitlement agent self-attaches over a Unix socket
# the HotSpot VM places at /tmp/.java_pid<pid> (the OS temp dir, NOT java.io.tmpdir).
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /conf:rw,mode=1777 \
  --tmpfs /data:rw,mode=1777 \
  --tmpfs /var/log/elasticsearch:rw,mode=1777 \
  --tmpfs /tmp:rw,mode=1777 \
  -p 127.0.0.1:9200:9200 "$IMAGE" >/dev/null

# Elasticsearch + JVM start takes a while; poll the HTTP API for the cluster JSON
ok=""
for i in $(seq 1 80); do
  if curl -fsS http://127.0.0.1:9200/ 2>/dev/null | grep -q '"cluster_name"'; then
    ok=1; break
  fi
  [ "$i" = 80 ] && { echo "elasticsearch did not become ready"; docker logs "$NAME" | tail -60; exit 1; }
  sleep 3
done
echo "API up:"; curl -fsS http://127.0.0.1:9200/ 2>/dev/null | grep -E '"number"|"cluster_name"' | head -2

echo "checking cluster health"
curl -fsS 'http://127.0.0.1:9200/_cluster/health' 2>/dev/null | grep -qE '"status":"(green|yellow)"' \
  || { echo "cluster not healthy"; exit 1; }

# index a doc and read it back
curl -fsS -XPUT 'http://127.0.0.1:9200/gate/_doc/1?refresh=true' -H 'Content-Type: application/json' -d '{"k":"quench"}' >/dev/null
curl -fsS 'http://127.0.0.1:9200/gate/_doc/1' 2>/dev/null | grep -q '"quench"' || { echo "index/get failed"; exit 1; }

# must run as the nonroot elasticsearch user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
