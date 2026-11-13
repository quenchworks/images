package smoke

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func init() {
	register("teleport", func(t *T) {
		user := t.User("1001")
		ver := t.Run("version")
		t.Logf("%s", strings.TrimSpace(ver))
		t.WantVersionV(ver)
		// teleport start has no --cluster-name or --config-string: mount a minimal v3 config.
		dir := t.TempDir()
		cfg := filepath.Join(dir, "teleport.yaml")
		os.WriteFile(cfg, []byte(`version: v3
teleport:
  nodename: quench
  data_dir: /var/lib/teleport
  log:
    output: stderr
    severity: INFO
auth_service:
  enabled: true
  cluster_name: quench.test
proxy_service:
  enabled: true
  web_listen_addr: 0.0.0.0:3080
  public_addr: localhost:3080
ssh_service:
  enabled: false
`), 0o644)
		name := t.Name("")
		t.Start(name, []string{"-p", "127.0.0.1:13080:3080", "--tmpfs", "/var/lib/teleport:uid=1001,gid=1001", "-v", cfg + ":/etc/teleport/teleport.yaml:ro"},
			"start", "--config=/etc/teleport/teleport.yaml")
		const b = "https://127.0.0.1:13080"
		get := func(path string, hdr ...string) (int, string) {
			req, _ := http.NewRequest("GET", b+path, nil)
			if len(hdr) == 2 {
				req.Header.Set(hdr[0], hdr[1])
			}
			return insecureDo(req)
		}
		var ping string
		t.waitFor(60, 3*time.Second, name, "proxy never answered /webapi/ping", func() bool {
			if s := t.Docker("inspect", "-f", "{{.State.Status}}", name); s != "running" {
				t.Fatalf("teleport exited\n%s", t.Logs(name))
			}
			_, ping = get("/webapi/ping")
			return strings.Contains(ping, `"server_version"`)
		})
		if t.Want != "" {
			t.Contains(ping, `"server_version":"`+t.Want+`"`, "ping version")
		}
		_, page := get("/web/login")
		t.Match(page, `<script[^>]+src="/web/app/`, "web UI bundle served")
		asset := t.Match(page, `(/web/app/[^"]+\.js)`, "UI asset path")
		// The bundle is embedded Brotli-only (app.js.br): ask the way a browser does.
		if c, _ := get(asset, "Accept-Encoding", "br"); c != 200 {
			t.Fatalf("UI asset %s answered %d", asset, c)
		}
		t.Contains(t.Docker("exec", name, "tctl", "--config=/etc/teleport/teleport.yaml", "status"), "quench.test", "tctl status")
		t.Logf("smoke test passed (teleport %s, uid %s, web UI served, tctl reached auth)", t.Want, user)
	})
	register("connaisseur", func(t *T) {
		dir := t.TempDir()
		for _, d := range []string{"certs", "config", "alerts"} {
			os.MkdirAll(filepath.Join(dir, d), 0o755)
		}
		crt, key := certPair("connaisseur")
		os.WriteFile(filepath.Join(dir, "certs/tls.crt"), crt, 0o644)
		os.WriteFile(filepath.Join(dir, "certs/tls.key"), key, 0o644)
		os.WriteFile(filepath.Join(dir, "config/config.yaml"), []byte(`validators:
  - name: allow
    type: static
    approve: true
  - name: deny
    type: static
    approve: false
policy:
  - pattern: "*:*"
    validator: deny
  - pattern: "docker.io/library/allowed:*"
    validator: allow
`), 0o644)
		os.WriteFile(filepath.Join(dir, "alerts/config.yaml"), []byte("{}\n"), 0o644) // an empty file fails to parse (EOF)
		// CACHE_EXPIRY_SECONDS=0: no Redis cache (otherwise it needs /app/redis-certs and a Redis).
		name := t.Name("")
		t.Start(name, []string{"-p", "127.0.0.1:5000:5000", "-e", "CACHE_EXPIRY_SECONDS=0", "-v", dir + "/certs:/app/certs:ro",
			"-v", dir + "/config:/app/config:ro", "-v", dir + "/alerts:/app/alerts:ro"})
		t.waitFor(30, time.Second, name, "connaisseur did not come up", func() bool {
			r, err := insecure.Get("https://127.0.0.1:5000/health")
			if err != nil {
				return false
			}
			r.Body.Close()
			return r.StatusCode == 200
		})
		for img, want := range map[string]string{"allowed:1.0": "true", "denied:1.0": "false"} {
			review := `{"apiVersion":"admission.k8s.io/v1","kind":"AdmissionReview","request":{"uid":"uid-` + img + `",
"kind":{"group":"","version":"v1","kind":"Pod"},"resource":{"group":"","version":"v1","resource":"pods"},"namespace":"default",
"operation":"CREATE","userInfo":{"username":"smoke"},"object":{"apiVersion":"v1","kind":"Pod","metadata":{"name":"p","namespace":"default"},
"spec":{"containers":[{"name":"c","image":"docker.io/library/` + img + `"}]}}}}`
			req, _ := http.NewRequest("POST", "https://127.0.0.1:5000/mutate", strings.NewReader(review))
			req.Header.Set("Content-Type", "application/json")
			_, out := insecureDo(req)
			if !strings.Contains(out, `"allowed":`+want) {
				t.Fatalf("%s: expected allowed=%s, got: %s\n%s", img, want, out, t.Logs(name))
			}
			t.Logf("%s -> allowed=%s", img, want)
		}
		t.Logf("smoke test passed (user %s)", t.User("1001"))
	})
	// Want is the OpenJDK major (CI passes 21, not 21.0.11).
	register("jre", func(t *T) {
		if t.Want == "" {
			t.Fatalf("usage: test.sh <image-ref> <major>")
		}
		m := t.Want
		ro := []string{"--read-only", "--tmpfs", "/tmp"}
		ver, _ := t.RunFlags(ro, "-version")
		t.Logf("%s", strings.TrimSpace(ver))
		t.Match(ver, `openjdk version "`+regexp.QuoteMeta(m)+`[."]`, "java -version major")
		// Runtime-only: the compiler and REPL must not be there.
		for _, tool := range []string{"javac", "jshell"} {
			if _, err := t.RunFlags(append(ro, "--entrypoint", "/usr/bin/"+tool), "-version"); err == nil {
				t.Fatalf("%s ran: this image is not runtime-only", tool)
			}
		}
		props, _ := t.RunFlags(append(ro, "--entrypoint", "/usr/bin/java"), "-XshowSettings:properties", "-version")
		t.Contains(t.Match(props, `java\.home = (.*)`, "java.home"), "java-"+m+"-openjdk", "java.home")
		mods, _ := t.RunFlags(append(ro, "--entrypoint", "/usr/bin/java"), "--list-modules")
		t.Contains(mods, "java.base@"+m+".", "java.base module")
		if strings.Contains(mods, "jdk.compiler") {
			t.Fatalf("jdk.compiler module present: not a pure JRE")
		}
		t.User("1001")
		t.Contains(t.Docker("inspect", t.Image, "--format", "{{json .Config.Entrypoint}}"), "/usr/bin/java", "entrypoint")
		t.Logf("smoke test passed (OpenJDK JRE %s, runtime-only, nonroot uid: 1001, read-only rootfs)", m)
	})
}
