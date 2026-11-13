#!/usr/bin/env bash
# Smoke test for a built Kanboard image. Usage: test.sh <image-ref> [version]
# Kanboard runs on a read-only root with data/ on a volume (SQLite). Through the JSON-RPC API, as
# the seeded admin: read the version, refuse a wrong password, create a project and a task, then
# restart the container and find them again. The login page must render through nginx + php-fpm.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="kanboard-smoke-$$"
URL=http://127.0.0.1:18082
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; docker volume rm "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
docker volume create "$NAME" >/dev/null
start() {
  docker run -d --name "$NAME" --read-only --tmpfs /tmp:uid=1001,gid=1001,mode=1777 -v "$NAME:/var/www/app/data" \
    -p 127.0.0.1:18082:8080 "$IMAGE" >/dev/null
  for i in $(seq 1 60); do curl -fsS "$URL/healthcheck.php" >/dev/null 2>&1 && return 0
    [ "$i" = 60 ] && { echo "healthcheck never answered"; docker logs "$NAME" 2>&1 | tail -30; exit 1; }; sleep 1; done
}
rpc() { # $1 password, $2 method, $3 params (JSON)
  curl -sS -u "admin:$1" -H 'Content-Type: application/json' "$URL/jsonrpc.php" \
    -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"$2\",\"params\":${3:-{\}}}"; }
result() { jq -ec 'if has("error") then halt_error(1) else .result end'; }
start
curl -fsS "$URL/?controller=AuthController&action=login" | grep -qi 'password' || { echo "login page not rendered"; exit 1; }
ver="$(rpc admin getVersion | result | tr -d '"')"
echo "kanboard $ver"
[ -z "$WANT" ] || [ "$ver" = "v$WANT" ] || [ "$ver" = "$WANT" ] || { echo "expected $WANT"; exit 1; }
code="$(curl -s -o /dev/null -w '%{http_code}' -u admin:wrong -H 'Content-Type: application/json' "$URL/jsonrpc.php" -d '{"jsonrpc":"2.0","id":1,"method":"getVersion"}')"
[ "$code" = 401 ] || { echo "a wrong password got HTTP $code"; exit 1; }
pid="$(rpc admin createProject '{"name":"quench-smoke"}' | result)"
rpc admin createTask "{\"title\":\"quench-smoke-task\",\"project_id\":$pid}" | result >/dev/null
echo "  version ok, wrong password refused, project $pid + task created"
docker rm -f "$NAME" >/dev/null
start
rpc admin getAllTasks "{\"project_id\":$pid}" | grep -q quench-smoke-task || { echo "task lost across a restart"; exit 1; }
echo "  project and task kept across a restart"
if docker logs "$NAME" 2>&1 | grep -qiE 'read-only file system|PHP Fatal'; then echo "error in the logs"; docker logs "$NAME" 2>&1 | tail -20; exit 1; fi
echo "smoke test passed (kanboard ${WANT:-?}: nginx + php-fpm, JSON-RPC admin auth, project and task kept on the data volume, read-only root; user $user)"
