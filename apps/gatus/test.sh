#!/usr/bin/env bash
# Smoke test for a built Gatus image. Usage: test.sh <image-ref> [version]
# Gatus monitors two endpoints: our caddy image (must report up) and a port nothing listens on
# (must report down). The status API must show both results, and /health must answer.
# Upstream stamps no version, so the version argument is not checked.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
TAG="gatus-smoke-$$"
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
cleanup() { docker rm -f "$TAG" "$TAG-up" >/dev/null 2>&1 || true; docker network rm "$TAG" >/dev/null 2>&1 || true; rm -rf "$WORK"; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
cat > "$WORK/config.yaml" <<YML
endpoints:
  - name: up-backend
    url: http://$TAG-up:8000/
    interval: 5s
    conditions: ["[STATUS] == 200", "[BODY] == quench-up"]
  - name: down-backend
    url: http://$TAG-up:9999/
    interval: 5s
    client: { timeout: 2s }
    conditions: ["[STATUS] == 200"]
YML
chmod -R a+rX "$WORK"
docker network create "$TAG" >/dev/null
docker run -d --name "$TAG-up" --network "$TAG" --entrypoint /usr/bin/caddy ghcr.io/quenchworks/images/caddy:2.11.9 \
  respond --listen :8000 -b quench-up >/dev/null
docker run -d --name "$TAG" --network "$TAG" --read-only -v "$WORK:/config:ro" -p 127.0.0.1:18080:8080 "$IMAGE" >/dev/null
out=""
for _ in $(seq 1 30); do
  out="$(curl -fsS http://127.0.0.1:18080/api/v1/endpoints/statuses 2>/dev/null || true)"
  grep -q '"up-backend"' <<<"$out" && grep -q '"down-backend"' <<<"$out" && grep -q '"results":\[{' <<<"$out" && break
  sleep 2
done
python3 - "$out" <<'PY' || { docker logs "$TAG" 2>&1 | tail -15; exit 1; }
import json, sys
s = {e["name"]: e["results"][-1]["success"] for e in json.loads(sys.argv[1]) if e.get("results")}
print("latest results:", s)
assert s.get("up-backend") is True, "up-backend not reported healthy"
assert s.get("down-backend") is False, "down-backend not reported failing"
PY
curl -fsS http://127.0.0.1:18080/health | grep -qi up || { echo "/health did not answer UP"; exit 1; }
if docker logs "$TAG" 2>&1 | grep -qE 'panic|read-only file system'; then echo "crashed or wrote to the root"; exit 1; fi
echo "smoke test passed: healthy and failing endpoints reported, /health up, user 1001"
