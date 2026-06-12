#!/usr/bin/env bash
# Smoke test for a built Solr image. Usage: test.sh <image-ref>
# Runs with a READ-ONLY rootfs + writable tmpfs at /var/solr to prove the entrypoint
# relocates SOLR_HOME/logs/pid/tmp correctly, then exercises the HTTP API.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-solr-smoke-$$"
BASE="http://127.0.0.1:8983/solr"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE (read-only rootfs + writable /var/solr tmpfs)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /var/solr:rw,mode=1777 \
  -p 127.0.0.1:8983:8983 "$IMAGE" >/dev/null

# Solr + JVM start takes a while; poll the system info API for the lucene/solr version.
ok=""
for i in $(seq 1 60); do
  if curl -fsS "${BASE}/admin/info/system?wt=json" 2>/dev/null | grep -q '"lucene"'; then
    ok=1; break
  fi
  [ "$i" = 60 ] && { echo "solr did not become ready"; docker logs "$NAME" | tail -50; exit 1; }
  sleep 2
done
echo "API up:"; curl -fsS "${BASE}/admin/info/system?wt=json" 2>/dev/null \
  | grep -oE '"solr-spec-version":"[^"]*"|"lucene-spec-version":"[^"]*"' | head -2

# create a core
echo "creating core 'gate'"
curl -fsS "${BASE}/admin/cores?action=CREATE&name=gate&configSet=_default" >/dev/null \
  || { echo "core create failed"; docker logs "$NAME" | tail -40; exit 1; }

# index a doc and commit
echo "indexing a doc"
curl -fsS -XPOST "${BASE}/gate/update?commit=true" \
  -H 'Content-Type: application/json' \
  -d '[{"id":"1","title_s":"quench"}]' >/dev/null \
  || { echo "index failed"; exit 1; }

# query it back
echo "querying"
curl -fsS "${BASE}/gate/select?q=id:1" 2>/dev/null | grep -q '"quench"' \
  || { echo "query did not return the indexed doc"; exit 1; }

# must run as the nonroot solr user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
