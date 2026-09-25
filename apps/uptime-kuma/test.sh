#!/usr/bin/env bash
# Smoke test for a built uptime-kuma image. Usage: test.sh <image-ref> [version]
# Boots the server on a read-only root with SQLite in a volume, waits for the
# migrations to reach the latest schema, and checks the UI, the API and that /metrics
# demands credentials.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="uptime-kuma-smoke-$$"
VOL="$NAME-data"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; docker volume rm "$VOL" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

ver="$(docker run --rm --entrypoint /usr/bin/node "$IMAGE" -p 'require("/usr/lib/uptime-kuma/package.json").version')"
echo "uptime-kuma $ver"
[ -z "$WANT" ] || [ "$ver" = "$WANT" ] || { echo "expected version $WANT"; exit 1; }

docker volume create "$VOL" >/dev/null
# a fresh named volume is root-owned; hand it to uid 1001 the way a pod fsGroup would
docker run --rm -u 0 -v "$VOL:/d" --entrypoint /usr/bin/node "$IMAGE" -e 'require("fs").chownSync("/d",1001,1001)'
docker run -d --name "$NAME" --read-only --tmpfs /tmp -v "$VOL:/app/data" \
  -p 127.0.0.1:13001:3001 "$IMAGE" >/dev/null
ok=0
for _ in $(seq 1 120); do
  curl -fsS http://127.0.0.1:13001/api/entry-page >/dev/null 2>&1 && { ok=1; break; }
  sleep 1
done
[ "$ok" = 1 ] || { echo "server never answered"; docker logs "$NAME" 2>&1 | tail -40; exit 1; }

curl -fsS http://127.0.0.1:13001/api/entry-page | grep -q '"type":"entryPage"' || { echo "entry-page API wrong"; exit 1; }
curl -fsSL http://127.0.0.1:13001/ | grep -qi '<div id="app"' || { echo "UI not served"; exit 1; }
code="$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:13001/metrics)"
[ "$code" = 401 ] || { echo "/metrics returned $code, expected 401"; exit 1; }
docker logs "$NAME" 2>&1 | grep -qi 'database type: sqlite' || { echo "not on sqlite"; docker logs "$NAME" 2>&1 | tail -30; exit 1; }
if docker logs "$NAME" 2>&1 | grep -E 'ERROR|Error:' | grep -v 'db-config.json is not found'; then
  echo "server logged errors"; exit 1
fi
echo "smoke test passed (uptime-kuma ${ver}, uid $user, sqlite in a volume, read-only root)"
