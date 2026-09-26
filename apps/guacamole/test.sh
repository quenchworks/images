#!/usr/bin/env bash
# Smoke test for a built Guacamole web app image. Usage: test.sh <image-ref> [version]
# Boots Tomcat with no database, then requires: the app at /guacamole/, the REST
# API answering, and the PostgreSQL extension loaded and reading its settings from
# the environment (it asks for postgresql-database). The chart gate logs in
# against a real database.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
NAME="quench-guacamole-smoke-$$"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

docker run -d --name "$NAME" --read-only --tmpfs /tmp -e GUACD_HOSTNAME=127.0.0.1 \
  -p 127.0.0.1:8080:8080 "$IMAGE" >/dev/null
for i in $(seq 1 60); do
  code="$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/guacamole/ || true)"
  [ "$code" = 200 ] && break
  [ "$i" = 60 ] && { echo "guacamole did not serve /guacamole/ (last $code)"; docker logs "$NAME" | tail -40; exit 1; }
  sleep 2
done
langs="$(curl -fsS http://127.0.0.1:8080/guacamole/api/languages)"
grep -q '"en":"English"' <<<"$langs" || { echo "languages API wrong: ${langs:0:200}"; exit 1; }
logs="$(docker logs "$NAME" 2>&1)"
grep -q 'Extension "PostgreSQL Authentication" (postgresql) loaded' <<<"$logs" || { echo "postgresql extension not loaded"; exit 1; }
grep -q 'Property postgresql-database is required' <<<"$logs" || { echo "extension did not read environment properties"; exit 1; }

echo "smoke test passed (web app, API, postgresql extension; nonroot user: $user)"
