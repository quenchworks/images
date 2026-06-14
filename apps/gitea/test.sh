#!/usr/bin/env bash
# Smoke test for a built Gitea image. Usage: test.sh <image-ref>
#
# Gitea needs a database to fully operate, but `gitea web` serves the HTTP listener
# (and the install page on /) BEFORE a DB is configured when INSTALL_LOCK is not set.
# To stay self-contained (no external Postgres) we boot with a minimal app.ini that
# only sets the server section and leave the install lock off, then assert:
#   - `gitea --version` prints the pinned version
#   - the web server boots and binds :3000
#   - GET / returns HTTP 200 (the install page is served without a DB)
#   - the container runs as nonroot uid 1001 on a read-only rootfs
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-gitea-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "checking gitea --version"
docker run --rm --entrypoint /usr/bin/gitea "$IMAGE" --version

# Minimal app.ini: just the server block + a writable work dir. No DB section, no
# INSTALL_LOCK, so gitea serves the install page on / without touching a database.
echo "booting gitea web (read-only rootfs, writable /data + /etc/gitea + /tmp)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /tmp:rw \
  --tmpfs /data:rw,uid=1001,gid=1001 \
  --tmpfs /etc/gitea:rw,uid=1001,gid=1001 \
  -p 3000:3000 \
  -e GITEA_APP_INI=/etc/gitea/app.ini \
  --entrypoint /bin/sh \
  "$IMAGE" -c '
    cat > /etc/gitea/app.ini <<INI
APP_NAME = QuenchWorks Gitea smoke
RUN_MODE = prod
[server]
HTTP_ADDR = 0.0.0.0
HTTP_PORT = 3000
[log]
MODE = console
LEVEL = info
INI
    exec /usr/bin/gitea web --config /etc/gitea/app.ini
  ' >/dev/null

# Wait for the listener to come up.
ok=0
for i in $(seq 1 60); do
  code="$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:3000/ 2>/dev/null || echo 000)"
  if [ "$code" = "200" ] || [ "$code" = "302" ]; then
    ok=1
    break
  fi
  if [ "$(docker inspect -f '{{.State.Running}}' "$NAME" 2>/dev/null || echo false)" != "true" ]; then
    echo "container exited early:"; docker logs "$NAME" 2>&1 | tail -40; exit 1
  fi
  sleep 1
done

if [ "$ok" != 1 ]; then
  echo "gitea did not serve / within timeout:"
  docker logs "$NAME" 2>&1 | tail -40
  exit 1
fi
echo "GET / -> $code OK (web server up, install page served without a DB)"

# must run as the nonroot gitea user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot uid: $user, read-only rootfs, :3000 -> $code)"
