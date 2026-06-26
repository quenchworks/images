#!/usr/bin/env bash
# Smoke test for a built MLflow Tracking Server image. Usage: test.sh <image-ref> [expected-version]
# mlflow-skinny is client-only; `mlflow server` needs a full set of server deps, so a
# `mlflow --version` check is NOT enough (it passes even when the server can't boot).
# This test BOOTS the actual tracking server (against an ephemeral SQLite backend store
# + local artifact root, no external Postgres) and asserts /health and the experiments
# API both return HTTP 200, as nonroot uid 1001. The chart's kind gate repeats this
# against a bundled Postgres.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
EXPECT_VER="${2:-}"

echo "checking the image runs as nonroot uid 1001"
USER_CFG="$(docker inspect --format '{{.Config.User}}' "$IMAGE")"
echo "  Config.User: $USER_CFG"
[ "$USER_CFG" = "1001" ] || { echo "FAIL: image user is not 1001"; exit 1; }

echo "mlflow --version (no DB required):"
VER_OUT="$(docker run --rm --entrypoint /usr/bin/mlflow "$IMAGE" --version)"
echo "$VER_OUT" | sed 's/^/  /'
echo "$VER_OUT" | grep -q '^mlflow, version ' || { echo "FAIL: no version line"; exit 1; }
if [ -n "$EXPECT_VER" ]; then
  echo "$VER_OUT" | grep -q "^mlflow, version ${EXPECT_VER}$" \
    || { echo "FAIL: expected mlflow, version ${EXPECT_VER}"; exit 1; }
  echo "  version matches ${EXPECT_VER}"
fi

# Boot the real tracking server. SQLite backend store + local artifact root live under
# /tmp (writable tmpfs) so no external Postgres is needed for the image-level smoke test.
# The container runs as the image user (uid 1001, asserted above via Config.User).
PORT=15000
CID="mlflow-smoke-$$"
echo "booting 'mlflow server' (sqlite backend, /tmp artifacts) on :${PORT}"
docker run -d --name "$CID" \
  --read-only --tmpfs /tmp \
  -p "127.0.0.1:${PORT}:5000" \
  "$IMAGE" \
  server --backend-store-uri "sqlite:////tmp/mlflow.db" \
         --default-artifact-root "/tmp/artifacts" \
         --host 0.0.0.0 --port 5000 >/dev/null

cleanup() { docker rm -f "$CID" >/dev/null 2>&1 || true; }
trap cleanup EXIT

probe() { # $1 = url path ; echoes HTTP status or ERR
  curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:${PORT}$1" 2>/dev/null || echo ERR
}

echo "waiting for /health to return 200"
HEALTH=ERR
for _ in $(seq 1 60); do
  HEALTH="$(probe /health)"
  [ "$HEALTH" = "200" ] && break
  if [ -z "$(docker ps -q -f "name=^${CID}$")" ]; then
    echo "FAIL: server container exited during boot"; docker logs "$CID" 2>&1 | tail -40; exit 1
  fi
  sleep 1
done
echo "  /health: $HEALTH"
[ "$HEALTH" = "200" ] || { echo "FAIL: /health did not return 200"; docker logs "$CID" 2>&1 | tail -40; exit 1; }

API="$(probe '/api/2.0/mlflow/experiments/search?max_results=10')"
echo "  /api/2.0/mlflow/experiments/search: $API"
[ "$API" = "200" ] || { echo "FAIL: experiments API did not return 200"; docker logs "$CID" 2>&1 | tail -40; exit 1; }

echo "PASS: mlflow server smoke test green (/health 200, experiments API 200, uid 1001)"
