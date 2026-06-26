#!/usr/bin/env bash
# Smoke test for a built MLflow Tracking Server image. Usage: test.sh <image-ref> [expected-version]
# A full `mlflow server` boot needs a reachable backend store (Postgres) and is exercised
# by the chart's kind-install gate, not here. For the IMAGE we confirm: it runs as nonroot
# uid 1001, and `mlflow --version` (needs no DB) prints the expected MLflow version.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
EXPECT_VER="${2:-}"

echo "checking the image runs as nonroot uid 1001"
USER_CFG="$(docker inspect --format '{{.Config.User}}' "$IMAGE")"
echo "  Config.User: $USER_CFG"
[ "$USER_CFG" = "1001" ] || { echo "FAIL: image user is not 1001"; exit 1; }

echo "mlflow --version (no DB required), run read-only as the image user:"
VER_OUT="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" --version)"
echo "$VER_OUT" | sed 's/^/  /'
echo "$VER_OUT" | grep -q '^mlflow, version ' || { echo "FAIL: no version line"; exit 1; }

if [ -n "$EXPECT_VER" ]; then
  echo "$VER_OUT" | grep -q "^mlflow, version ${EXPECT_VER}$" \
    || { echo "FAIL: expected mlflow, version ${EXPECT_VER}"; exit 1; }
  echo "  version matches ${EXPECT_VER}"
fi

echo "PASS: mlflow smoke test green"
