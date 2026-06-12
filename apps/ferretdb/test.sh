#!/usr/bin/env bash
# Smoke test for a built FerretDB image. Usage: test.sh <image-ref>
# FerretDB needs a DocumentDB-extended PostgreSQL to serve data, which is out of
# scope for an image smoke test. We assert the binary reports its version, the
# process comes up and serves its liveness probe, and it runs nonroot.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-ferretdb-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "checking --version"
ver="$(docker run --rm "$IMAGE" --version 2>/dev/null | grep '^version:')"
echo "  $ver"
printf '%s' "$ver" | grep -q 'v2.7.0' || { echo "unexpected version: $ver"; exit 1; }

echo "starting server (no backend; liveness must still come up)"
docker run -d --name "$NAME" -p 127.0.0.1:8088:8088 "$IMAGE" >/dev/null

# wait for the debug server to serve the liveness probe
for i in $(seq 1 30); do
  if curl -fsS http://127.0.0.1:8088/debug/livez >/dev/null 2>&1; then
    break
  fi
  [ "$i" = 30 ] && { echo "livez did not come up"; docker logs "$NAME" | tail -20; exit 1; }
  sleep 1
done
echo "livez ok"

# must run as the nonroot ferretdb user (uid 1001). The image has no shell by
# design, so check the configured user rather than exec'ing `id`.
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
