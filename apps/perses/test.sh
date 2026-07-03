#!/usr/bin/env bash
# Smoke test for a built Perses image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

echo "perses version:"
out="$(docker run --rm --entrypoint /usr/bin/perses "$IMAGE" version 2>&1)"
echo "$out"
# version must be stamped (built from the tag, not an empty/dev version)
echo "$out" | grep -qiE 'v?[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "version not stamped"; exit 1; }

# must run as the nonroot perses user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

# start the server on a temp host port and curl the UI (HTTP 200).
name="perses-smoke-$$"
docker run -d --rm --name "$name" -p 18080:8080 "$IMAGE" >/dev/null
cleanup() { docker rm -f "$name" >/dev/null 2>&1 || true; }
trap cleanup EXIT

ok=0
for _ in $(seq 1 40); do
  code="$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:18080/ 2>/dev/null || true)"
  if [ "$code" = "200" ]; then ok=1; break; fi
  sleep 1
done
[ "$ok" = "1" ] || { echo "server did not serve / with HTTP 200"; docker logs "$name" 2>&1 | tail -30; exit 1; }

# the readiness/health endpoint should also answer.
health="$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:18080/api/health 2>/dev/null || true)"
echo "GET /api/health -> $health"

echo "smoke test passed (nonroot user: $user, / served HTTP 200)"
