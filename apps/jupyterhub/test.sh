#!/usr/bin/env bash
# Smoke test for a built JupyterHub hub image. Usage: test.sh <image-ref> [version]
# Starts the hub with its proxy external (the chart runs configurable-http-proxy
# as its own pod), a stand-in proxy API in its place, and requires the hub API to
# answer with this version, the hub to register its route with the proxy, and the
# KubeSpawner and OAuthenticator plugins to import.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="quench-jupyterhub-smoke-$$"
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$WORK"; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

ver="$(docker run --rm "$IMAGE" --version)"
echo "jupyterhub $ver"
[ -z "$WANT" ] || [ "$ver" = "$WANT" ] || { echo "expected $WANT"; exit 1; }
docker run --rm --entrypoint /opt/jupyterhub/venv/bin/python "$IMAGE" -c \
  "import kubespawner, oauthenticator; print('plugins import')"

# The config file is Python the hub executes, so it also starts a stand-in for
# configurable-http-proxy's REST API (routes kept in a dict) on 127.0.0.1:8001;
# the hub checks and adds its routes there at startup.
cat > "$WORK/jupyterhub_config.py" <<'PY'
import json, threading
from http.server import BaseHTTPRequestHandler, HTTPServer
ROUTES = {}
class Proxy(BaseHTTPRequestHandler):
    def _reply(self, code, body=b""):
        self.send_response(code); self.send_header("Content-Type", "application/json")
        self.end_headers(); self.wfile.write(body)
    def do_GET(self): self._reply(200, json.dumps(ROUTES).encode())
    def do_POST(self):
        n = int(self.headers.get("Content-Length", 0))
        ROUTES[self.path[len("/api/routes"):] or "/"] = json.loads(self.rfile.read(n) or b"{}")
        self._reply(201)
    def do_DELETE(self):
        ROUTES.pop(self.path[len("/api/routes"):] or "/", None); self._reply(204)
    def log_message(self, *a): pass
threading.Thread(target=HTTPServer(("127.0.0.1", 8001), Proxy).serve_forever, daemon=True).start()
c.JupyterHub.hub_ip = "0.0.0.0"
c.JupyterHub.hub_port = 8081
c.ConfigurableHTTPProxy.should_start = False
c.ConfigurableHTTPProxy.api_url = "http://127.0.0.1:8001"
c.ConfigurableHTTPProxy.auth_token = "smoke-token"
c.JupyterHub.authenticator_class = "dummy"
PY
chmod -R a+rX "$WORK"
docker run -d --name "$NAME" -p 127.0.0.1:8081:8081 -v "$WORK:/etc/jupyterhub:ro" \
  "$IMAGE" -f /etc/jupyterhub/jupyterhub_config.py >/dev/null
for i in $(seq 1 45); do
  api="$(curl -fsS http://127.0.0.1:8081/hub/api 2>/dev/null || true)"
  grep -q '"version"' <<<"$api" && break
  [ "$i" = 45 ] && { echo "hub API never answered"; docker logs "$NAME" | tail -30; exit 1; }
  sleep 1
done
echo "hub api: $api"
docker logs "$NAME" 2>&1 | grep -q "Adding route for Hub" || { echo "hub did not register its route with the proxy"; exit 1; }
[ -z "$WANT" ] || grep -q "\"version\": *\"$WANT\"" <<<"$api" || { echo "hub API reports another version"; exit 1; }

echo "smoke test passed (jupyterhub ${WANT:-?}: hub API, plugins; nonroot user: $user)"
