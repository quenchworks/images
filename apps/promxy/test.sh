#!/usr/bin/env bash
# Smoke test for a built Promxy image. Usage: test.sh <image-ref> [version]
# Promxy fronts one Prometheus (our image, scraping itself). A PromQL query through Promxy
# must return that Prometheus' up series, and the Prometheus API shape must hold.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
TAG="promxy-smoke-$$"
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
cleanup() { docker rm -f "$TAG" "$TAG-prom" >/dev/null 2>&1 || true; docker network rm "$TAG" >/dev/null 2>&1 || true; rm -rf "$WORK"; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
v="$(docker run --rm "$IMAGE" --version 2>&1)"
echo "$v" | head -1
[ -z "$WANT" ] || grep -qF "version $WANT" <<<"$v" || { echo "expected $WANT"; exit 1; }

cat > "$WORK/promxy.yaml" <<YML
promxy:
  server_groups:
    - static_configs:
        - targets: ["$TAG-prom:9090"]
YML
chmod -R a+rX "$WORK"
docker network create "$TAG" >/dev/null
docker run -d --name "$TAG-prom" --network "$TAG" --tmpfs /prometheus:uid=1001,gid=1001 ghcr.io/quenchworks/images/prometheus:3.16.0 >/dev/null
docker run -d --name "$TAG" --network "$TAG" --read-only -v "$WORK:/c:ro" -p 127.0.0.1:18082:8082 "$IMAGE" \
  --config=/c/promxy.yaml --bind-addr=:8082 >/dev/null
out=""
for _ in $(seq 1 60); do
  out="$(curl -fsS 'http://127.0.0.1:18082/api/v1/query?query=up' 2>/dev/null || true)"
  grep -q '"__name__":"up"' <<<"$out" && break
  sleep 2
done
grep -q '"status":"success"' <<<"$out" && grep -q '"__name__":"up"' <<<"$out" \
  || { echo "up not returned through promxy: $out"; docker logs "$TAG" 2>&1 | tail -15; exit 1; }
echo "query through promxy: $(grep -o '"metric":{[^}]*}' <<<"$out" | head -1)"
if docker logs "$TAG" 2>&1 | grep -qE 'panic|read-only file system'; then echo "crashed or wrote to the root"; exit 1; fi
echo "smoke test passed: PromQL through promxy reaches the Prometheus server group, user 1001"
