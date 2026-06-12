#!/usr/bin/env bash
# Smoke test for a built Neo4j Community image. Usage: test.sh <image-ref>
# Runs with a READ-ONLY rootfs + writable tmpfs mounts to prove the entrypoint
# relocates data/logs/run/conf/tmp correctly, then exercises the HTTP discovery API
# and runs a Cypher round-trip via the in-image cypher-shell.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-neo4j-smoke-$$"
PASS="quenchtest123"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE (read-only rootfs + writable tmpfs for data/logs/conf/run)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /data:rw,exec,mode=1777 \
  --tmpfs /logs:rw,mode=1777 \
  --tmpfs /conf:rw,mode=1777 \
  --tmpfs /var/run/neo4j:rw,mode=1777 \
  -e NEO4J_AUTH="neo4j/${PASS}" \
  -p 127.0.0.1:7474:7474 -p 127.0.0.1:7687:7687 "$IMAGE" >/dev/null

# Neo4j + JVM start takes a while; poll the HTTP discovery endpoint (returns 200 with a
# JSON pointing at the Bolt endpoint -- this is what a kube httpGet probe on 7474 hits).
ok=""
for i in $(seq 1 90); do
  if curl -fsS http://127.0.0.1:7474/ 2>/dev/null | grep -q '"bolt'; then
    ok=1; break
  fi
  [ "$i" = 90 ] && { echo "neo4j did not become ready"; docker logs "$NAME" | tail -60; exit 1; }
  sleep 2
done
echo "HTTP discovery up:"; curl -fsS http://127.0.0.1:7474/ 2>/dev/null | head -c 300; echo

# HTTP discovery on / must be 200 WITHOUT auth (the kube probe is unauthenticated).
code="$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:7474/)"
[ "$code" = "200" ] || { echo "expected HTTP 200 unauthenticated on /, got $code"; exit 1; }

# Cypher round-trip via the in-image cypher-shell (Bolt 7687).
echo "running Cypher write + read via cypher-shell"
docker exec "$NAME" cypher-shell -a bolt://localhost:7687 -u neo4j -p "$PASS" \
  "CREATE (:Thing {name:'quench'}) RETURN 1;" >/dev/null \
  || { echo "cypher write failed"; docker logs "$NAME" | tail -40; exit 1; }

out="$(docker exec "$NAME" cypher-shell -a bolt://localhost:7687 -u neo4j -p "$PASS" \
  "MATCH (t:Thing) RETURN t.name;")"
echo "$out" | grep -q 'quench' || { echo "cypher read did not return 'quench': $out"; exit 1; }
echo "Cypher round-trip ok: $(echo "$out" | grep quench)"

# must run as the nonroot neo4j user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
