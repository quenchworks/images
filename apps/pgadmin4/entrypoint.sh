#!/bin/sh
# Creates or upgrades the config DB (and the first user, from PGADMIN_DEFAULT_EMAIL and
# PGADMIN_DEFAULT_PASSWORD or PGADMIN_DEFAULT_PASSWORD_FILE), then runs gunicorn.
set -eu
cd /opt/pgadmin/web
if [ -n "${PGADMIN_DEFAULT_PASSWORD_FILE:-}" ]; then
  PGADMIN_DEFAULT_PASSWORD="$(cat "$PGADMIN_DEFAULT_PASSWORD_FILE")"
fi
: "${PGADMIN_DEFAULT_EMAIL:?set PGADMIN_DEFAULT_EMAIL (the first user's login)}"
: "${PGADMIN_DEFAULT_PASSWORD:?set PGADMIN_DEFAULT_PASSWORD or PGADMIN_DEFAULT_PASSWORD_FILE}"
# servers.json is loaded on the first start only: load-servers adds duplicates on a rerun
first=0; [ -f "${PGADMIN_DATA_DIR:-/var/lib/pgadmin}/pgadmin4.db" ] || first=1
PGADMIN_SETUP_EMAIL="$PGADMIN_DEFAULT_EMAIL" PGADMIN_SETUP_PASSWORD="$PGADMIN_DEFAULT_PASSWORD" \
  python setup.py setup-db
if [ "$first" = 1 ] && [ -n "${PGADMIN_SERVER_JSON_FILE:-}" ] && [ -f "$PGADMIN_SERVER_JSON_FILE" ]; then
  python setup.py load-servers "$PGADMIN_SERVER_JSON_FILE" --user "$PGADMIN_DEFAULT_EMAIL"
fi
exec gunicorn --bind "0.0.0.0:${PGADMIN_LISTEN_PORT:-8080}" --workers 1 \
  --threads "${GUNICORN_THREADS:-25}" --timeout "${GUNICORN_TIMEOUT:-120}" \
  --worker-tmp-dir /tmp --access-logfile - --config gunicorn_config.py "$@" run_pgadmin:app
