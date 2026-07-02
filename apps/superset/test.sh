#!/usr/bin/env bash
# Smoke test for a built Apache Superset image. Usage: test.sh <image-ref> [expected-version]
#
# Superset CANNOT fully boot without a metadata DB (PostgreSQL) + Redis + a
# SUPERSET_SECRET_KEY, so this image-level smoke is deliberately limited (full DB-backed
# readiness is gated by the CHART's kind install, which bundles Postgres + Redis). Here
# we verify, WITHOUT a database:
#   1. the image runs as nonroot uid 1001,
#   2. `superset version` reports the version we built,
#   3. Python can import the superset core package AND `superset.app` (the app factory
#      module) — catching missing-module defects at import time (Day-13 lesson).
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
EXPECT_VER="${2:-}"

echo "checking the image runs as nonroot uid 1001"
USER_CFG="$(docker inspect --format '{{.Config.User}}' "$IMAGE")"
echo "  Config.User: $USER_CFG"
[ "$USER_CFG" = "1001" ] || { echo "FAIL: image user is not 1001"; exit 1; }

echo "superset version (no DB required):"
# `superset version` prints an ASCII banner + the version; run as the image user (1001).
# A dummy SECRET_KEY keeps the CLI from warning-exiting on some lines.
VER_OUT="$(docker run --rm --user 1001 -e SUPERSET_SECRET_KEY=smoketest-not-a-real-key \
           --entrypoint /opt/superset/venv/bin/superset "$IMAGE" version 2>/dev/null || true)"
echo "$VER_OUT" | tail -3 | sed 's/^/  /'
if [ -n "$EXPECT_VER" ]; then
  echo "$VER_OUT" | grep -qF "$EXPECT_VER" \
    || { echo "FAIL: expected version $EXPECT_VER in 'superset version' output"; exit 1; }
  echo "  version contains ${EXPECT_VER}"
else
  echo "$VER_OUT" | grep -qE '[0-9]+\.[0-9]+\.[0-9]+' \
    || { echo "FAIL: no version number in 'superset version' output"; exit 1; }
fi

echo "python can import superset core + superset.app (no DB), uid 1001:"
# import superset.app exercises the app-factory import path (blueprints, config,
# extensions) without opening a DB connection. The `superset` module has NO
# `__version__` attribute — read the version from package metadata via importlib.
IMPORT_OUT="$(docker run --rm --user 1001 \
  -e SUPERSET_SECRET_KEY=smoketest-not-a-real-key \
  --entrypoint /opt/superset/venv/bin/python "$IMAGE" \
  -c 'import importlib.metadata as m; import superset, superset.app; print("superset", m.version("apache-superset"), "import OK (superset + superset.app)")' 2>&1)" \
  || { echo "$IMPORT_OUT" | tail -25 | sed 's/^/  /'; echo "FAIL: could not import superset / superset.app"; exit 1; }
echo "$IMPORT_OUT" | tail -3 | sed 's/^/  /'
echo "$IMPORT_OUT" | grep -q 'import OK' \
  || { echo "FAIL: superset import did not report OK"; exit 1; }
if [ -n "$EXPECT_VER" ]; then
  echo "$IMPORT_OUT" | grep -qF "$EXPECT_VER" \
    || { echo "FAIL: imported apache-superset metadata version != ${EXPECT_VER}"; exit 1; }
fi

echo "PASS: superset image smoke green (uid 1001, superset version, superset + superset.app import; full DB-backed boot deferred to chart kind gate)"
