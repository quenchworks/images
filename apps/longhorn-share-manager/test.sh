#!/usr/bin/env bash
# Smoke test for a built longhorn-share-manager image. Usage: test.sh <image-ref> [version]
# The Go daemon needs a Longhorn volume device, so the test drives the part this image
# builds from source: ganesha.nfsd from Longhorn's fork, with the share-manager's config
# shape (NFSv4 only, RecoveryBackend = longhorn) and a VFS export of a tmpfs. A stub
# answers the fork's recovery backend (longhorn-manager's longhorn-recovery-backend:9503
# in a cluster); the test requires ganesha to register there, read its client list,
# listen on 2049 and stay up.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="quench-lhsm-smoke-$$"
STUB="quench-lhsm-recovery-$$"
NET="quench-lhsm-net-$$"
cleanup() { docker rm -f "$NAME" "$STUB" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "0" ] || [ -z "$user" ] || { echo "expected root like upstream, got '$user'"; exit 1; }

# The CLI has no --version flag; the build stamps main.Version into the binary.
[ -z "$WANT" ] || docker run --rm --entrypoint grep "$IMAGE" -aq "v$WANT" /longhorn-share-manager || { echo "binary not stamped v$WANT"; exit 1; }
docker run --rm "$IMAGE" daemon --help >/dev/null

docker network create "$NET" >/dev/null
docker run -d --name "$STUB" --network "$NET" --network-alias longhorn-recovery-backend python:3.12-slim python -u -c '
from http.server import BaseHTTPRequestHandler, HTTPServer
class H(BaseHTTPRequestHandler):
    def answer(self):
        n = int(self.headers.get("Content-Length") or 0)
        if n: self.rfile.read(n)
        print("CALL", self.command, self.path, flush=True)
        body = b"{\"clients\": []}"
        self.send_response(200); self.send_header("Content-Length", str(len(body))); self.end_headers(); self.wfile.write(body)
    do_GET = do_POST = do_PUT = do_DELETE = answer
    def log_message(self, *a): pass
HTTPServer(("0.0.0.0", 9503), H).serve_forever()' >/dev/null

docker run -d --name "$NAME" --network "$NET" --cap-add DAC_READ_SEARCH --cap-add SYS_RESOURCE \
  --tmpfs /export/vol:exec --entrypoint sh "$IMAGE" -c '
cat > /tmp/vfs.conf <<CONF
NFS_CORE_PARAM { Protocols = 4; }
NFSV4 { Lease_Lifetime = 60; Grace_Period = 90; Minor_Versions = 0, 1, 2; RecoveryBackend = longhorn; Only_Numeric_Owners = true; }
LOG { Default_Log_Level = INFO; Facility { name = FILE; destination = "/tmp/ganesha.log"; enable = active; } }
EXPORT { Export_Id = 1; Path = /export/vol; Pseudo = /vol; Protocols = 4; Transports = TCP; Access_Type = RW; SecType = sys; Squash = No_Root_Squash; FSAL { Name = VFS; } }
CONF
exec ganesha.nfsd -F -p /var/run/ganesha.pid -f /tmp/vfs.conf' >/dev/null

for i in $(seq 1 30); do
  st="$(docker inspect -f '{{.State.Status}}' "$NAME")"
  if [ "$st" = exited ] || [ "$st" = dead ]; then
    echo "ganesha.nfsd exited"; docker logs "$NAME" 2>&1 | tail -20; docker cp "$NAME:/tmp/ganesha.log" - 2>/dev/null | tail -c 3000; exit 1
  fi
  ports="$(docker exec "$NAME" netstat -ltn 2>/dev/null || true)"
  grep -q ':2049 ' <<<"$ports" && break
  [ "$i" = 30 ] && { echo "nothing listens on 2049"; docker exec "$NAME" tail -30 /tmp/ganesha.log; exit 1; }
  sleep 1
done
sleep 3
log="$(docker exec "$NAME" cat /tmp/ganesha.log)"
[ "$(docker inspect -f '{{.State.Status}}' "$NAME")" = running ] || { echo "ganesha.nfsd died after binding"; tail -30 <<<"$log"; exit 1; }
grep -q 'NFS SERVER INITIALIZED' <<<"$log" || { echo "ganesha did not finish initializing"; tail -30 <<<"$log"; exit 1; }
calls="$(docker logs "$STUB" 2>&1)"
grep -q 'CALL POST /v1/recoverybackend' <<<"$calls" || { echo "ganesha never registered with the recovery backend"; echo "$calls"; exit 1; }
grep -q 'CALL GET /v1/recoverybackend/' <<<"$calls" || { echo "ganesha never read its recovery client list"; echo "$calls"; exit 1; }
echo "smoke test passed (longhorn-share-manager ${WANT:-?}: ganesha (Longhorn fork) registers with the recovery backend, serves NFSv4 on 2049; user ${user:-0})"
