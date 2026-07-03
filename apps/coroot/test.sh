#!/usr/bin/env bash
# Smoke test for a built Coroot image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

# must run as the nonroot coroot user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

# Coroot has no `version` subcommand (version is logged at startup), so we start
# the server and assert it (a) logs a stamped, non-"unknown" version and (b)
# serves its embedded UI on :8080 with HTTP 200.
name="coroot-smoke-$$"
docker run -d --rm --name "$name" -p 18080:8080 "$IMAGE" >/dev/null
cleanup() { docker rm -f "$name" >/dev/null 2>&1 || true; }
trap cleanup EXIT

ok=0
for _ in $(seq 1 60); do
  code="$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:18080/ 2>/dev/null || true)"
  if [ "$code" = "200" ]; then ok=1; break; fi
  sleep 1
done
[ "$ok" = "1" ] || { echo "server did not serve / with HTTP 200"; docker logs "$name" 2>&1 | tail -30; exit 1; }

# version must be stamped from the tag (not the default "unknown")
logs="$(docker logs "$name" 2>&1)"
echo "$logs" | grep -qiE 'version:[[:space:]]*[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "version not stamped (expected 'version: X.Y.Z'):"; echo "$logs" | grep -i version || true; exit 1; }

echo "smoke test passed (nonroot user: $user, / served HTTP 200, version stamped)"
