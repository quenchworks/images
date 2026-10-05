#!/usr/bin/env bash
# Smoke test for a built support-bundle-kit image. Usage: test.sh <image-ref> [version]
# Runs the node agent the way the support bundle manager's DaemonSet does
# (support-bundle-collector.sh with the longhorn collector, the container's own root standing in
# for the host), and requires a stub manager to receive node_bundle.zip with the collected
# host information in it.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="quench-sbk-smoke-$$"
STUB="quench-sbk-manager-$$"
NET="quench-sbk-net-$$"
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
cleanup() { docker rm -f "$NAME" "$STUB" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; rm -rf "$WORK"; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "0" ] || [ -z "$user" ] || { echo "expected root like upstream, got '$user'"; exit 1; }

ver="$(docker run --rm "$IMAGE" support-bundle-kit version)"
echo "$ver"
[ -z "$WANT" ] || grep -q "^v$WANT " <<<"$ver" || { echo "expected v$WANT"; exit 1; }

docker network create "$NET" >/dev/null
chmod 777 "$WORK"
docker run -d --name "$STUB" --network "$NET" --network-alias sb-manager -v "$WORK:/out" python:3.12-slim python -u -c '
from http.server import BaseHTTPRequestHandler, HTTPServer
class H(BaseHTTPRequestHandler):
    def do_POST(self):
        n = int(self.headers.get("Content-Length") or 0)
        open("/out/node_bundle.zip", "wb").write(self.rfile.read(n))
        print("GOT", self.path, n, flush=True)
        self.send_response(200); self.send_header("Content-Length", "0"); self.end_headers()
    def log_message(self, *a): pass
srv = HTTPServer(("0.0.0.0", 8080), H)
print("READY", flush=True)
srv.serve_forever()' >/dev/null
for i in $(seq 1 60); do grep -q READY <<<"$(docker logs "$STUB" 2>&1)" && break; [ "$i" = 60 ] && { echo "stub never started"; exit 1; }; sleep 1; done

docker run -d --name "$NAME" --network "$NET" -e SUPPORT_BUNDLE_MANAGER_URL=http://sb-manager:8080 \
  -e SUPPORT_BUNDLE_COLLECTOR=longhorn -e SUPPORT_BUNDLE_NODE_NAME=smoke-node \
  "$IMAGE" support-bundle-collector.sh >/dev/null
for i in $(seq 1 60); do
  grep -q 'GOT /nodes/smoke-node' <<<"$(docker logs "$STUB" 2>&1)" && break
  [ "$(docker inspect -f '{{.State.Status}}' "$NAME")" = exited ] && { echo "collector exited before uploading"; docker logs "$NAME" 2>&1 | tail -30; exit 1; }
  [ "$i" = 60 ] && { echo "the manager never received a node bundle"; docker logs "$NAME" 2>&1 | tail -30; exit 1; }
  sleep 1
done
listing="$(python3 -c 'import sys,zipfile; print("\n".join(zipfile.ZipFile(sys.argv[1]).namelist()))' "$WORK/node_bundle.zip")"
grep -q '^smoke-node/hostinfos/hostinfo$' <<<"$listing" || { echo "bundle lacks hostinfos/hostinfo:"; head -20 <<<"$listing"; exit 1; }
if docker logs "$NAME" 2>&1 | grep -q 'No such file or directory.*common'; then echo "collector could not source its common functions"; exit 1; fi

echo "smoke test passed (support-bundle-kit ${WANT:-?}: longhorn node collector bundled $(wc -l <<<"$listing") entries and uploaded them; user ${user:-0})"
