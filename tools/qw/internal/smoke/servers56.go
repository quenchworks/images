package smoke

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// The hub's config is Python the hub itself executes (inside the image), so it also starts a
// stand-in for configurable-http-proxy's REST API on 127.0.0.1:8001; the hub registers its route there.
const jupyterhubConfig = `import json, threading
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
`

func init() {
	// The chart runs configurable-http-proxy as its own pod, so the hub starts with the proxy external.
	register("jupyterhub", func(t *T) {
		user := t.User("1001")
		ver := strings.TrimSpace(t.Run("--version"))
		t.Logf("jupyterhub %s", ver)
		if t.Want != "" && ver != t.Want {
			t.Fatalf("expected %s", t.Want)
		}
		t.Contains(t.RunEntry("/opt/jupyterhub/venv/bin/python", "-c", "import kubespawner, oauthenticator; print('plugins import')"), "plugins import", "KubeSpawner and OAuthenticator import")
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "jupyterhub_config.py"), []byte(jupyterhubConfig), 0o644)
		os.Chmod(dir, 0o755)
		name, api := t.httpUp([]string{"-p", "127.0.0.1:8081:8081", "-v", dir + ":/etc/jupyterhub:ro"}, "http://127.0.0.1:8081/hub/api", 45,
			"-f", "/etc/jupyterhub/jupyterhub_config.py")
		t.Logf("hub api: %s", strings.TrimSpace(api))
		// The hub serves its API before it logs the proxy route.
		t.waitFor(20, time.Second, name, "hub did not register its route with the proxy", func() bool {
			return strings.Contains(t.Logs(name), "Adding route for Hub")
		})
		if t.Want != "" && !regexp.MustCompile(`"version": *"`+regexp.QuoteMeta(t.Want)+`"`).MatchString(api) {
			t.Fatalf("hub API reports another version: %s", api)
		}
		t.Logf("smoke test passed (jupyterhub %s: hub API, proxy route, plugins; nonroot user: %s)", ver, user)
	})
}

func init() {
	// The operator installs Knative from the release manifests in its kodata, so the image must
	// carry them where KO_DATA_PATH points; against a dead API server both binaries must start
	// and fail at the dial.
	register("knative-operator", func(t *T) {
		u := t.User("1001")
		t.LabelVersion()
		t.Contains(t.Docker("inspect", t.Image, "--format", "{{json .Config.Env}}"), "KO_DATA_PATH=/usr/share/knative-operator/kodata", "KO_DATA_PATH")
		if n := len(t.Files("usr/share/knative-operator/kodata/knative-serving/")); n < 10 {
			t.Fatalf("only %d files under /usr/share/knative-operator/kodata/knative-serving: the release manifests are missing", n)
		}
		kc := t.DeadKubeconfig()
		for _, b := range []string{"/operator", "/webhook"} {
			out, _ := t.RunAnyFor(60*time.Second, []string{"--read-only", "--tmpfs", "/tmp", "--entrypoint", b, "-v", kc + ":/w/kubeconfig:ro",
				"-e", "KUBECONFIG=/w/kubeconfig", "-e", "SYSTEM_NAMESPACE=knative-operator", "-e", "POD_NAME=smoke", "-e", "WEBHOOK_NAME=operator-webhook"}, "--kubeconfig=/w/kubeconfig")
			// the first API call is the config-logging ConfigMap read; its error does not name the address
			t.Match(out, `127\.0\.0\.1:1|Error reading/parsing logging configuration`, b+" reached the API server step")
		}
		t.Logf("smoke test passed (knative-operator %s: kodata present, operator and webhook reach the API server; nonroot user %s)", t.Want, u)
	})
}
