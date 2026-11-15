package smoke

import (
	"crypto/tls"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func init() {
	// On a read-only rootfs with server.toml, a CA-signed TLS pair and /data on a volume: kanidmd serves
	// /status over HTTPS, recover-account issues an admin password, and the database (and the admin)
	// is still there after a restart on the same volume.
	register("kanidm", func(t *T) {
		u := t.User("1001")
		t.Contains(t.RunAny("version"), t.Want, "kanidmd version")
		dir, err := os.MkdirTemp(envOr("TMPDIR", ""), "kanidm-smoke-")
		if err != nil {
			t.Fatalf("temp dir: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(dir) })
		_ = os.Chmod(dir, 0o755)
		caAndServer(dir, "localhost")
		srv, _ := os.ReadFile(filepath.Join(dir, "serverCert.pem"))
		ca, _ := os.ReadFile(filepath.Join(dir, "caCert.pem"))
		cfg := `version = "2"
bindaddress = "0.0.0.0:8443"
db_path = "/data/kanidm.db"
tls_chain = "/etc/kanidm/chain.pem"
tls_key = "/etc/kanidm/serverKey.pem"
domain = "localhost"
origin = "https://localhost:18443"
`
		for name, body := range map[string][]byte{"server.toml": []byte(cfg), "chain.pem": append(srv, ca...)} {
			if err := os.WriteFile(filepath.Join(dir, name), body, 0o644); err != nil {
				t.Fatalf("%v", err)
			}
		}
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		cl := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
		status := func() (int, string) {
			r, err := cl.Get("https://127.0.0.1:18443/status")
			if err != nil {
				return 0, err.Error()
			}
			defer r.Body.Close()
			b, _ := io.ReadAll(r.Body)
			return r.StatusCode, string(b)
		}
		start := func(name string) {
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", dir + ":/etc/kanidm:ro", "-v", vol + ":/data",
				"-p", "127.0.0.1:18443:8443"})
			t.waitFor(90, time.Second, name, "kanidmd never answered /status", func() bool {
				if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
					t.Fatalf("kanidmd exited\n%s", t.Logs(name))
				}
				c, b := status()
				return c == 200 && strings.Contains(b, "true")
			})
		}
		recover := func(name string) string {
			out, err := docker("exec", name, "/usr/bin/kanidmd", "recover-account", "admin")
			if err != nil {
				t.Fatalf("recover-account admin: %v\n%s", err, out)
			}
			return t.Match(out, `(?i)(?:new_)?password[":\s]+"?([A-Za-z0-9]{10,})`, "recovered admin password")
		}
		name := t.Name("")
		start(name)
		recover(name)
		docker("rm", "-f", name)
		again := t.Name("again")
		start(again)
		recover(again)
		t.Logf("smoke test passed (kanidm %s: read-only boot, HTTPS /status, admin recovered, database kept across a restart; user %s)", t.Want, u)
	})
}
