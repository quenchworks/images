#!/usr/bin/env bash
# Smoke test for a built gogs image. Usage: test.sh <image-ref> [version]
# Real Git service work on a read-only root with /data on a volume: Gogs starts from a
# locked config (SQLite, built-in SSH), an admin is created from the CLI, a repository
# is auto-initialised through the API (git in the image), and a clone plus push over
# HTTP runs the server-side hooks (bash in the image) and shows up through the API.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="gogs-smoke-$$"
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
trap 'docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$WORK"' EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
ver="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" --version)"
[ -z "$WANT" ] || echo "$ver" | grep -qF "$WANT" || { echo "version mismatch: $ver"; exit 1; }

cat > "$WORK/app.ini" <<CONF
RUN_USER = git
RUN_MODE = prod
[server]
HTTP_PORT = 3000
EXTERNAL_URL = http://127.0.0.1:13000/
START_SSH_SERVER = true
SSH_PORT = 2222
SSH_LISTEN_PORT = 2222
[database]
TYPE = sqlite3
PATH = /data/gogs.db
[repository]
ROOT = /data/repositories
[security]
INSTALL_LOCK = true
SECRET_KEY = smoke-$$-$RANDOM$RANDOM
[log]
MODE = console
LEVEL = Info
CONF
chmod -R a+rX "$WORK"
# exec on /data: the hooks Gogs writes into each repository run from there
docker run -d --name "$NAME" --read-only --tmpfs /tmp --tmpfs /data:exec,uid=1001,gid=1001 \
  -v "$WORK/app.ini:/etc/gogs/app.ini:ro" -p 127.0.0.1:13000:3000 "$IMAGE" web --config /etc/gogs/app.ini >/dev/null
URL=http://127.0.0.1:13000
ok=0
for _ in $(seq 1 60); do
  curl -fsS "$URL/healthcheck" 2>/dev/null | grep -q 'Database connection: OK' && { ok=1; break; }
  sleep 1
done
[ "$ok" = 1 ] || { echo "gogs never became healthy"; docker logs "$NAME" 2>&1 | tail -30; exit 1; }
docker logs "$NAME" 2>&1 | grep -q 'SSH server started' || { echo "built-in SSH server did not start"; docker logs "$NAME" 2>&1 | tail -30; exit 1; }

PW="Quench-smoke-$$"
docker exec "$NAME" /usr/bin/gogs admin create-user --name quench --password "$PW" --email smoke@example.com --admin --config /etc/gogs/app.ini >/dev/null
tok="$(curl -fsS -u "quench:$PW" -H 'Content-Type: application/json' -d '{"name":"smoke"}' "$URL/api/v1/users/quench/tokens" | sed -n 's/.*"sha1":"\([0-9a-f]*\)".*/\1/p')"
[ -n "$tok" ] || { echo "no API token"; exit 1; }
curl -fsS -o /dev/null -H "Authorization: token $tok" -H 'Content-Type: application/json' \
  -d '{"name":"smoke","auto_init":true,"readme":"Default"}' "$URL/api/v1/user/repos"
curl -fsS -H "Authorization: token $tok" "$URL/api/v1/repos/quench/smoke/raw/master/README.md" | grep -q '# smoke' \
  || { echo "auto-init README missing"; exit 1; }

git clone -q "http://quench:$PW@127.0.0.1:13000/quench/smoke.git" "$WORK/clone"
( cd "$WORK/clone" && echo quench > pushed.txt && git add pushed.txt \
  && git -c user.email=smoke@example.com -c user.name=smoke commit -qm push && git push -q origin master )
curl -fsS -H "Authorization: token $tok" "$URL/api/v1/repos/quench/smoke/raw/master/pushed.txt" | grep -qx quench \
  || { echo "pushed file not served"; docker logs "$NAME" 2>&1 | tail -20; exit 1; }
if docker logs "$NAME" 2>&1 | grep -E '\[ERROR\]|\[FATAL\]|panic:'; then echo "gogs logged errors"; exit 1; fi
echo "smoke test passed (gogs ${WANT:-?}, uid $user, repo created, clone and push through the hooks)"
