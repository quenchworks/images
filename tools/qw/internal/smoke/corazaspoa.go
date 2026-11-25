package smoke

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func init() {
	// End to end with the catalog's HAProxy in front, wired as upstream's example: a clean request reaches a host stub
	// (200), an XSS probe and a SQL injection probe are denied by the CRS (403), and the agent itself runs read-only as
	// 1001 on the shipped config.
	register("coraza-spoa", func(t *T) {
		u := t.User("1001")
		t.Contains(t.Run("--version"), "v"+t.Want, "coraza-spoa --version")
		stub := t.hostStub(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, "backend ok") }))
		net_ := t.Name("net")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		spoa := t.Name("")
		// Coraza buffers request bodies in /tmp and checks it is writable at start
		t.Start(spoa, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--network", net_, "--network-alias", "spoa"})
		// it logs no listen address; the HAProxy round trip below proves it serves on 9000
		t.waitFor(30, time.Second, spoa, "the agent never started", func() bool {
			return strings.Contains(t.Logs(spoa), "Starting coraza-spoa")
		})
		d, err := os.MkdirTemp("", "coraza-smoke")
		if err != nil {
			t.Fatalf("temp dir: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(d) })
		os.Chmod(d, 0o755)
		backend := strings.TrimPrefix(stub, "http://")
		cfg := map[string]string{
			"haproxy.cfg": `global
  log stdout format raw local0
defaults
  log global
  timeout client 30s
  timeout server 30s
  timeout connect 5s
frontend web
  mode http
  bind :8080
  http-request set-var(txn.coraza.app) str(default)
  filter spoe engine coraza config /etc/haproxy/coraza.cfg
  http-request send-spoe-group coraza coraza-req
  http-request deny deny_status 403 if { var(txn.coraza.action) -m str deny }
  http-request deny deny_status 500 if { var(txn.coraza.error) -m int gt 0 }
  default_backend app
backend app
  mode http
  server app ` + backend + `
backend coraza-spoa
  mode tcp
  server spoa spoa:9000
`,
			"coraza.cfg": `[coraza]
spoe-agent coraza-agent
  groups coraza-req
  option var-prefix coraza
  option set-on-error error
  timeout hello 2s
  timeout idle 2m
  timeout processing 2s
  use-backend coraza-spoa
spoe-message coraza-req
  args app=var(txn.coraza.app) src-ip=src src-port=src_port dst-ip=dst dst-port=dst_port method=method path=path query=query version=req.ver headers=req.hdrs body=req.body exportRuleIDs=bool(false)
spoe-group coraza-req
  messages coraza-req
`,
		}
		for n, b := range cfg {
			if err := os.WriteFile(filepath.Join(d, n), []byte(b), 0o644); err != nil {
				t.Fatalf("write %s: %v", n, err)
			}
		}
		hp := t.Name("haproxy")
		t.Cleanup(func() { docker("rm", "-f", hp) })
		t.Docker(append(append([]string{"run", "-d", "--name", hp, "--network", net_, "-p", "127.0.0.1:18085:8080",
			"-v", d + ":/etc/haproxy:ro"}, hostStubFlag...), "ghcr.io/quenchworks/images/haproxy:3.4.0")...)
		code := func(q string) int {
			c, _ := restDo("GET", "http://127.0.0.1:18085/"+q, "", "", "")
			return c
		}
		t.waitFor(30, time.Second, hp, "haproxy never served the clean request", func() bool { return code("?q=hello") == 200 })
		for _, probe := range []string{"?q=%3Cscript%3Ealert(1)%3C%2Fscript%3E", "?id=1%27%20OR%20%271%27%3D%271"} {
			if c := code(probe); c != 403 {
				t.Fatalf("probe %s answered %d, want 403\n%s\n%s", probe, c, t.Logs(hp), t.Logs(spoa))
			}
		}
		t.Logf("smoke test passed (coraza-spoa %s: behind HAProxy, clean request 200, XSS and SQLi probes 403 from the CRS; user %s)", t.Want, u)
	})
}
