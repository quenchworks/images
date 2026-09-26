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
# --version only. 2026.8 removed the `version` subcommand, and cobra ignores an
# unknown positional arg on the root command, so `authentik-server version` starts
# the server and waits for PostgreSQL forever (it hung this boot test for 50
# minutes on both arches). rootCmd sets Version:, so --version prints and exits.
VER_OUT="$(docker run --rm --entrypoint /usr/bin/authentik-server "$IMAGE" --version)"
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

echo "django.setup() populates the FULL INSTALLED_APPS registry (no DB required):"
# Regression guard for the uv-workspace editable-install bug: authentik's
# settings.py lists workspace-local packages (django_channels_postgres et al.)
# in INSTALLED_APPS unconditionally. `ak dump_config` above only imports the
# config module and does NOT populate Django's app registry, so it MISSED that
# those packages were installed as dangling editable stubs (.pth -> build-time
# source path absent from the runtime image). django.setup() imports EVERY
# INSTALLED_APPS module, so any missing/dangling workspace package fails here,
# pre-publish. This reaches app-registry population without touching Postgres/
# Redis (no live connection at import time). Run as the image user (1001).
SETUP_OUT="$(docker run --rm --user 1001 \
  -e AUTHENTIK_SECRET_KEY=smoketest \
  -e DJANGO_SETTINGS_MODULE=authentik.root.settings \
  --entrypoint /ak-root/.venv/bin/python "$IMAGE" \
  -c 'import django; django.setup(); print("django.setup OK: INSTALLED_APPS registry populated")' 2>&1)" \
  || { echo "$SETUP_OUT" | tail -25 | sed 's/^/  /'; echo "FAIL: django.setup() could not populate the app registry (missing INSTALLED_APPS module?)"; exit 1; }
echo "$SETUP_OUT" | tail -3 | sed 's/^/  /'
echo "$SETUP_OUT" | grep -q 'django.setup OK' \
  || { echo "FAIL: django.setup() did not report success"; exit 1; }

# Belt-and-suspenders: import each uv-workspace-local package directly, so a
# regression is unambiguous even if a future settings refactor drops one from
# INSTALLED_APPS. Modules: guardian (ak-guardian), django_channels_postgres,
# django_dramatiq_postgres, django_postgres_cache.
echo "importing every uv-workspace-local package directly (uid 1001):"
WS_OUT="$(docker run --rm --user 1001 --entrypoint /ak-root/.venv/bin/python "$IMAGE" \
  -c 'import guardian, django_channels_postgres, django_dramatiq_postgres, django_postgres_cache; print("workspace packages import OK")' 2>&1)" \
  || { echo "$WS_OUT" | tail -15 | sed 's/^/  /'; echo "FAIL: a uv-workspace-local package is not importable"; exit 1; }
echo "$WS_OUT" | tail -1 | sed 's/^/  /'

echo "PASS: authentik image smoke green (uid 1001, authentik-server version, Python core imports, django.setup app registry + workspace packages; full boot deferred to chart kind gate)"
