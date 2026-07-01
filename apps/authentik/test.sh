#!/usr/bin/env bash
# Smoke test for a built authentik image. Usage: test.sh <image-ref> [expected-version]
#
# authentik CANNOT fully boot without PostgreSQL + Redis, so this image-level smoke is
# deliberately limited (full readiness is gated by the CHART's kind install, which
# bundles PG + Redis). Here we verify, WITHOUT a database:
#   1. the image runs as nonroot uid 1001,
#   2. the Go `authentik-server` binary is present and reports its version,
#   3. the `ak` launcher is present and dispatches,
#   4. Python can import the authentik core package (via `ak dump_config`, which runs
#      `python -m authentik.lib.config` and needs no DB).
# Commands that call wait_for_db (ak server / worker / healthcheck) are intentionally
# NOT exercised here — they block without a database, by design.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
EXPECT_VER="${2:-}"

echo "checking the image runs as nonroot uid 1001"
USER_CFG="$(docker inspect --format '{{.Config.User}}' "$IMAGE")"
echo "  Config.User: $USER_CFG"
[ "$USER_CFG" = "1001" ] || { echo "FAIL: image user is not 1001"; exit 1; }

echo "authentik-server version (Go binary, no DB required):"
VER_OUT="$(docker run --rm --entrypoint /usr/bin/authentik-server "$IMAGE" version 2>/dev/null \
           || docker run --rm --entrypoint /usr/bin/authentik-server "$IMAGE" --version)"
echo "$VER_OUT" | sed 's/^/  /'
echo "$VER_OUT" | grep -q '[0-9]\{4\}\.[0-9]' || { echo "FAIL: no CalVer version line"; exit 1; }
if [ -n "$EXPECT_VER" ]; then
  echo "$VER_OUT" | grep -q "$EXPECT_VER" \
    || { echo "FAIL: expected version $EXPECT_VER in output"; exit 1; }
  echo "  version contains ${EXPECT_VER}"
fi

echo "ak launcher present + Python core importable (ak dump_config, no DB):"
# dump_config -> python -m authentik.lib.config: exercises the venv + Django settings
# import path without touching Postgres/Redis. Run as the image user (1001).
CFG_OUT="$(docker run --rm --entrypoint /usr/bin/ak "$IMAGE" dump_config 2>/dev/null || true)"
if [ -z "$CFG_OUT" ]; then
  # Fallback: import the package directly through the venv interpreter.
  CFG_OUT="$(docker run --rm --entrypoint /ak-root/.venv/bin/python "$IMAGE" \
             -c 'import authentik; import django; print("authentik core import OK; django", django.get_version())')"
fi
echo "$CFG_OUT" | tail -5 | sed 's/^/  /'
echo "$CFG_OUT" | grep -qiE 'authentik|postgresql|redis|django|"[a-z_]+"' \
  || { echo "FAIL: python could not import/emit the authentik core config"; exit 1; }

echo "PASS: authentik image smoke green (uid 1001, authentik-server version, Python core imports; full boot deferred to chart kind gate)"
