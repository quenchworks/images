#!/usr/bin/env bash
# Smoke test for a built nginx-proxy-manager image. Usage: test.sh <image-ref> [version]
#
# A green admin page proves little, so this drives the product end to end:
#   1. Boots on a READ-ONLY rootfs as uid 1001, /data and /etc/letsencrypt in volumes,
#      the first admin account from INITIAL_ADMIN_EMAIL/INITIAL_ADMIN_PASSWORD.
#   2. GET /api/ on the admin port (8181) reports status OK and the release version.
#   3. Logs in through the API and creates a proxy host "smoke.test" forwarding to the
#      admin server: the backend renders the host config, runs `nginx -t` and reloads.
#   4. A request on the HTTP port (8080) with Host: smoke.test comes back from the
#      admin UI, through the new proxy host.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="npm-smoke-$$"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; docker volume rm "$NAME-data" "$NAME-le" >/dev/null 2>&1 || true; }
trap cleanup EXIT
free_port() { python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])'; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

for v in data le; do
  docker volume create "$NAME-$v" >/dev/null
  # a fresh named volume is root-owned; hand it to uid 1001 the way a pod fsGroup would
  docker run --rm -u 0 -v "$NAME-$v:/d" --entrypoint /usr/bin/node "$IMAGE" -e 'require("fs").chownSync("/d",1001,1001)'
done
ADMIN="$(free_port)"; HTTP="$(free_port)"
docker run -d --name "$NAME" --read-only --tmpfs /tmp \
  -v "$NAME-data:/data" -v "$NAME-le:/etc/letsencrypt" \
  -e INITIAL_ADMIN_EMAIL=admin@smoke.test -e INITIAL_ADMIN_PASSWORD=quench-smoke-pw \
  -e IP_RANGES_FETCH_ENABLED=false \
  -p "127.0.0.1:${ADMIN}:8181" -p "127.0.0.1:${HTTP}:8080" "$IMAGE" >/dev/null

api=""
for i in $(seq 1 120); do
  api="$(curl -fsS "http://127.0.0.1:${ADMIN}/api/" 2>/dev/null || true)"
  grep -q '"status":"OK"' <<<"$api" && break
  docker ps --filter "name=$NAME" --filter status=running --format '{{.Names}}' | grep -q "$NAME" \
    || { echo "container died:"; docker logs "$NAME"; exit 1; }
  [ "$i" = 120 ] && { echo "API never reported OK: $api"; docker logs "$NAME" | tail -40; exit 1; }
  sleep 1
done
echo "api: $api"
if [ -n "$WANT" ]; then
  IFS=. read -r ma mi re <<<"$WANT"
  grep -q "\"major\":$ma,\"minor\":$mi,\"revision\":$re" <<<"$api" || { echo "expected version $WANT"; exit 1; }
fi

token="$(curl -fsS -H 'Content-Type: application/json' \
  -d '{"identity":"admin@smoke.test","secret":"quench-smoke-pw"}' \
  "http://127.0.0.1:${ADMIN}/api/tokens" | python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])')"
created="$(curl -sS -H "Authorization: Bearer $token" -H 'Content-Type: application/json' \
  -d '{"domain_names":["smoke.test"],"forward_scheme":"http","forward_host":"127.0.0.1","forward_port":8181,"meta":{}}' \
  "http://127.0.0.1:${ADMIN}/api/nginx/proxy-hosts")"
echo "created: ${created:0:200}"
id="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["id"])' "$created")"

# the backend writes the config, tests it and reloads nginx after answering; it records
# the outcome in the host's meta
body=""
for i in $(seq 1 30); do
  host="$(curl -fsS -H "Authorization: Bearer $token" "http://127.0.0.1:${ADMIN}/api/nginx/proxy-hosts/$id" || true)"
  body="$(curl -sS -H 'Host: smoke.test' "http://127.0.0.1:${HTTP}/" 2>/dev/null || true)"
  grep -q '"nginx_online":true' <<<"$host" && grep -qi '<div id="root"' <<<"$body" && break
  [ "$i" = 30 ] && { echo "proxy host not online and serving: ${host:0:300} / ${body:0:200}"; docker logs "$NAME" | tail -40; exit 1; }
  sleep 1
done
echo "Host: smoke.test on :8080 served the admin UI through the new proxy host"

docker exec "$NAME" certbot --version
if docker logs "$NAME" 2>&1 | grep -qiE 'read-only file system|permission denied|EACCES|\[emerg\]'; then
  echo "errors in the logs:"; docker logs "$NAME"; exit 1
fi
echo "smoke test passed (nonroot $user, read-only boot, API login, proxy host created and serving)"
