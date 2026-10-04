#!/usr/bin/env bash
# Smoke test for a built longhorn-ui image. Usage: test.sh <image-ref> [version]
# Starts the image as upstream's chart does (uid 499, LONGHORN_MANAGER_IP set) with a stub
# manager behind it, and requires the UI's index and bundle on 8000 and /v1 proxied through.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="quench-lhui-smoke-$$"
STUB="quench-lhui-manager-$$"
NET="quench-lhui-net-$$"
PORT=18000
cleanup() { docker rm -f "$NAME" "$STUB" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
trap cleanup EXIT
fail() { echo "FAIL: $*"; docker logs "$NAME" 2>&1 | tail -20; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "499" ] || { echo "expected uid 499 like upstream, got '$user'"; exit 1; }

docker network create "$NET" >/dev/null
docker run -d --name "$STUB" --network "$NET" --network-alias longhorn-backend python:3.12-slim python -u -c '
from http.server import BaseHTTPRequestHandler, HTTPServer
class H(BaseHTTPRequestHandler):
    def do_GET(self):
        body = b"{\"type\": \"collection\", \"from\": \"stub-manager\"}"
        self.send_response(200); self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body))); self.end_headers(); self.wfile.write(body)
    def log_message(self, *a): pass
srv = HTTPServer(("0.0.0.0", 9500), H)
print("READY", flush=True)
srv.serve_forever()' >/dev/null
for i in $(seq 1 60); do grep -q READY <<<"$(docker logs "$STUB" 2>&1)" && break; [ "$i" = 60 ] && { echo "stub never started"; exit 1; }; sleep 1; done

docker run -d --name "$NAME" --network "$NET" -p "127.0.0.1:$PORT:8000" \
  -e LONGHORN_MANAGER_IP=http://longhorn-backend:9500 "$IMAGE" >/dev/null
for i in $(seq 1 30); do
  html="$(curl -fsS "http://127.0.0.1:$PORT/" 2>/dev/null || true)"
  grep -qi '<html' <<<"$html" && break
  [ "$(docker inspect -f '{{.State.Status}}' "$NAME")" = exited ] && fail "nginx exited"
  [ "$i" = 30 ] && fail "no index on 8000"
  sleep 1
done
# The index loads ./main.<hash>.async.js?<build-hash>; fetch the main bundle it names.
js="$(grep -oE '"\./main\.[0-9a-f]+(\.async)?\.js' <<<"$html" | head -1 | cut -c4- || true)"
[ -n "$js" ] || fail "index references no script"
bundle="$(curl -fsS "http://127.0.0.1:$PORT/$js")" || fail "bundle $js not served"
[ -z "$WANT" ] || docker exec "$NAME" grep -rqs "v$WANT" /web/dist || fail "bundle not stamped v$WANT"
v1="$(curl -fsS "http://127.0.0.1:$PORT/v1")" || fail "/v1 not proxied"
grep -q stub-manager <<<"$v1" || fail "/v1 did not reach the manager: $v1"
[ "${#bundle}" -gt 10000 ] || fail "bundle is suspiciously small (${#bundle} bytes)"

echo "smoke test passed (longhorn-ui ${WANT:-?}: index + bundle on 8000, /v1 proxied to the manager; uid $user)"
