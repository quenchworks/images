package smoke

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// certPair returns a self-signed certificate and its key in PEM, valid for dns.
func certPair(dns string) (cert, key []byte) {
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: dns}, DNSNames: []string{dns},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, _ := x509.CreateCertificate(rand.Reader, tpl, tpl, &k.PublicKey, k)
	kd, _ := x509.MarshalECPrivateKey(k)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kd})
}

func init() {
	register("elixir", func(t *T) {
		if t.Want == "" {
			t.Fatalf("usage: qw smoke elixir <image-ref> <minor>")
		}
		minor := strings.Join(strings.SplitN(t.Want, ".", 3)[:2], ".")
		ro := []string{"--read-only", "--tmpfs", "/tmp"}
		v, _ := t.RunFlags(ro, "--version")
		t.Contains(v, "Elixir "+minor+".", "version")
		if strings.Contains(strings.ToLower(v), "native name encoding of latin1") {
			t.Fatalf("latin1 warning present; UTF-8 not configured")
		}
		if out, _ := t.RunFlags(ro, "-e", `IO.puts("quench-elixir " <> Integer.to_string(1 + 1))`); out != "quench-elixir 2" {
			t.Fatalf("-e output wrong: '%s'", out)
		}
		if uid, _ := t.RunFlags(ro, "-e", `IO.puts(:os.cmd(~c"id -u") |> List.to_string() |> String.trim())`); uid != "1001" {
			t.Fatalf("expected uid 1001, got '%s'", uid)
		}
		t.User("1001")
		w := t.TempDir()
		os.WriteFile(filepath.Join(w, "h.exs"), []byte("IO.puts(\"quench-elixir-script\")\n"), 0o644)
		if out, _ := t.RunFlags(append(ro, "-v", w+":/work:ro", "-w", "/work"), "/work/h.exs"); out != "quench-elixir-script" {
			t.Fatalf(".exs output wrong: '%s'", out)
		}
		if _, err := t.RunFlags(ro, "-e", `case File.write("/should-fail", "x") do :ok -> System.halt(0); _ -> System.halt(1) end`); err == nil {
			t.Fatalf("rootfs was writable, expected read-only")
		}
		t.Logf("smoke test passed (Elixir %s, read-only rootfs)", minor)
	})
	register("istio-proxyv2", func(t *T) {
		u := t.User("1001")
		sha := regexp.MustCompile(`(?m)^ISTIO_META_ISTIO_PROXY_SHA=([0-9a-f]{40})$`).FindStringSubmatch(
			t.Docker("inspect", t.Image, "--format", "{{range .Config.Env}}{{println .}}{{end}}"))
		if sha == nil {
			t.Fatalf("ISTIO_META_ISTIO_PROXY_SHA is not a commit")
		}
		t.Contains(t.RunEntryAny("/usr/local/bin/envoy", "--version"), " version: "+sha[1]+"/", "envoy is the pinned Istio proxy build")
		m := regexp.MustCompile(`(?m)^client version: (\S+)`).FindStringSubmatch(strings.ReplaceAll(t.RunEntryAny("/usr/local/bin/pilot-agent", "version", "-s"), "\r", ""))
		if m == nil || strings.HasPrefix(m[1], "unknown") || (t.Want != "" && m[1] != t.Want) {
			t.Fatalf("pilot-agent version %v, expected %s", m, t.Want)
		}
		cid := t.Docker("create", t.Image)
		t.Cleanup(func() { docker("rm", "-f", cid) })
		p := filepath.Join(t.TempDir(), "tmpl.json")
		t.Docker("cp", cid+":/var/lib/istio/envoy/envoy_bootstrap_tmpl.json", p)
		b, _ := os.ReadFile(p)
		tmpl := string(b)
		t.Contains(tmpl, `"dynamic_resources"`, "Istio bootstrap template")
		name := t.Name("")
		t.Start(name, []string{"-p", "127.0.0.1:15000:15000", "--entrypoint", "/usr/local/bin/envoy"},
			"--config-yaml", "{admin: {address: {socket_address: {address: 0.0.0.0, port_value: 15000}}}}")
		info := t.WaitHTTP("http://127.0.0.1:15000/server_info", 30, name)
		t.Contains(info, `"state": "LIVE"`, "envoy LIVE")
		t.Contains(info, sha[1], "server_info commit")
		t.Logf("smoke test passed (pilot-agent %s, envoy %s LIVE, nonroot user %s)", m[1], sha[1], u)
	})
	register("kanboard", func(t *T) {
		const b = "http://127.0.0.1:18082"
		u := t.User("1001")
		vol, name := t.Name("data"), t.Name("")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		start := func() {
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:uid=1001,gid=1001,mode=1777", "-v", vol + ":/var/www/app/data", "-p", "127.0.0.1:18082:8080"})
			t.WaitHTTP(b+"/healthcheck.php", 60, name)
		}
		rpc := func(pw, method, params string) (int, json.RawMessage) {
			c, body := DoH("POST", "http://admin:"+pw+"@127.0.0.1:18082/jsonrpc.php",
				`{"jsonrpc":"2.0","id":1,"method":"`+method+`","params":`+params+`}`, map[string]string{"Content-Type": "application/json"})
			var r struct {
				Result json.RawMessage
				Error  json.RawMessage
			}
			if json.Unmarshal([]byte(body), &r) != nil || r.Error != nil {
				return c, nil
			}
			return c, r.Result
		}
		start()
		_, login := Get(b + "/?controller=AuthController&action=login")
		t.ContainsFold(login, "password", "login page")
		_, vr := rpc("admin", "getVersion", "{}")
		v := strings.Trim(string(vr), `"`)
		if v == "" || (t.Want != "" && v != t.Want && v != "v"+t.Want) {
			t.Fatalf("kanboard %q, expected %s", v, t.Want)
		}
		if c, _ := rpc("wrong", "getVersion", "{}"); c != 401 {
			t.Fatalf("a wrong password got HTTP %d", c)
		}
		_, pid := rpc("admin", "createProject", `{"name":"quench-smoke"}`)
		if pid == nil {
			t.Fatalf("createProject failed")
		}
		if _, r := rpc("admin", "createTask", `{"title":"quench-smoke-task","project_id":`+string(pid)+`}`); r == nil {
			t.Fatalf("createTask failed")
		}
		t.Docker("rm", "-f", name)
		start()
		_, tasks := rpc("admin", "getAllTasks", `{"project_id":`+string(pid)+`}`)
		t.Contains(string(tasks), "quench-smoke-task", "task kept across a restart")
		if regexp.MustCompile(`(?i)read-only file system|PHP Fatal`).MatchString(t.Logs(name)) {
			t.Fatalf("error in the logs\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed (kanboard %s: JSON-RPC auth, project and task kept on the data volume; user %s)", v, u)
	})
	register("mealie", func(t *T) {
		const b = "http://127.0.0.1:19000"
		u := t.User("1001")
		vol, name := t.Name("data"), t.Name("")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		start := func() {
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:uid=1001,gid=1001", "-v", vol + ":/app/data", "-p", "127.0.0.1:19000:9000"})
			t.WaitHTTP(b+"/api/app/about", 120, name)
		}
		login := func(pw string) (int, string) {
			c, body := Post(b+"/api/auth/token", "application/x-www-form-urlencoded", url.Values{"username": {"changeme@example.com"}, "password": {pw}}.Encode())
			var r struct {
				AccessToken string `json:"access_token"`
			}
			json.Unmarshal([]byte(body), &r)
			return c, r.AccessToken
		}
		start()
		_, ab := Get(b + "/api/app/about")
		var about struct{ Version string }
		json.Unmarshal([]byte(ab), &about)
		if about.Version == "" || (t.Want != "" && strings.TrimPrefix(about.Version, "v") != t.Want) {
			t.Fatalf("mealie %q, expected %s", about.Version, t.Want)
		}
		_, fe := Get(b + "/")
		t.ContainsFold(fe, "<html", "frontend served")
		if c, _ := login("wrong"); c != 401 {
			t.Fatalf("a wrong password got HTTP %d", c)
		}
		_, tok := login("MyPassword")
		_, slug := DoH("POST", b+"/api/recipes", `{"name":"Quench Smoke Soup"}`, map[string]string{"Authorization": "Bearer " + tok, "Content-Type": "application/json"})
		slug = strings.Trim(slug, `"`)
		if slug == "" {
			t.Fatalf("no recipe slug")
		}
		recipe := func() string {
			_, tok := login("MyPassword")
			_, r := DoH("GET", b+"/api/recipes/"+slug, "", map[string]string{"Authorization": "Bearer " + tok})
			return r
		}
		t.Contains(recipe(), "Quench Smoke Soup", "recipe readable")
		t.Docker("rm", "-f", name)
		start()
		t.Contains(recipe(), "Quench Smoke Soup", "recipe kept across a restart")
		if regexp.MustCompile(`(?i)read-only file system|Traceback`).MatchString(t.Logs(name)) {
			t.Fatalf("error in the logs\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed (mealie %s: login, recipe kept across a restart, frontend; user %s)", about.Version, u)
	})
	register("mysqld-exporter", func(t *T) {
		if v := t.RunAny("--version"); t.Want != "" {
			t.Contains(v, t.Want, "version")
		}
		_, m := t.httpUp([]string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-e", "MYSQLD_EXPORTER_PASSWORD=smoke", "-p", "127.0.0.1:9104:9104"},
			"http://127.0.0.1:9104/metrics", 30, "--mysqld.username=exporter")
		t.Contains(m, "mysqld_exporter_build_info", "/metrics")
		if t.Want != "" {
			t.Match(m, `mysqld_exporter_build_info\{[^}]*version="`+regexp.QuoteMeta(t.Want)+`"`, "build_info version")
		}
		t.Logf("smoke test passed (nonroot uid: %s, read-only rootfs)", t.User("1001"))
	})
	register("ollama", func(t *T) {
		u := t.User("1001")
		name, vb := t.httpUp([]string{"--read-only", "--tmpfs", "/models:rw,mode=1777", "-p", "127.0.0.1:11434:11434"}, "http://127.0.0.1:11434/api/version", 60)
		var v struct{ Version string }
		json.Unmarshal([]byte(vb), &v)
		if v.Version == "" || v.Version == "0.0.0" || strings.Contains(v.Version, "dev") {
			t.Fatalf("version not stamped: '%s'", v.Version)
		}
		t.WaitHTTP("http://127.0.0.1:11434/api/tags", 5, name)
		cli := regexp.MustCompile(`version is ([0-9]\S*)`).FindStringSubmatch(t.Docker("exec", name, "ollama", "--version"))
		if cli == nil || cli[1] != v.Version {
			t.Fatalf("cli version %v != server %s", cli, v.Version)
		}
		t.Logf("smoke test passed (version %s, nonroot user: %s)", v.Version, u)
	})
	register("opa-gatekeeper", func(t *T) {
		u := t.User("1001")
		w := t.TempDir()
		certs := filepath.Join(w, "certs")
		os.Mkdir(certs, 0o755)
		crt, key := certPair("gatekeeper-webhook-service.gatekeeper-system.svc")
		os.WriteFile(filepath.Join(certs, "tls.crt"), crt, 0o644)
		os.WriteFile(filepath.Join(certs, "ca.crt"), crt, 0o644)
		os.WriteFile(filepath.Join(certs, "tls.key"), key, 0o644)
		kc := t.DeadKubeconfig()
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp", "-v", certs + ":/certs:ro", "-v", kc + ":/etc/kubeconfig:ro",
			"-e", "KUBECONFIG=/etc/kubeconfig", "-e", "POD_NAMESPACE=gatekeeper-system"}, "--disable-cert-rotation")
		done := regexp.MustCompile(`"setting up audit"|unable to register controllers`)
		var out string
		t.waitFor(120, 2*time.Second, name, "controller setup never finished", func() bool { out = t.Logs(name); return done.MatchString(out) })
		t.Contains(out, `"starting manager"`, "manager started")
		if t.Want != "" {
			t.Contains(out, "gatekeeper/v"+t.Want, "version stamped")
		}
		if strings.Contains(out, "unable to create client cert watcher") {
			t.Fatalf("certificates not loaded")
		}
		t.Contains(out, `"setting up controllers"`, "controller setup")
		t.Contains(out, "127.0.0.1:1", "dialed the configured API server")
		if m := regexp.MustCompile(`panic:|read-only file system`).FindString(out); m != "" {
			t.Fatalf("manager crashed or wrote to the root: %s", m)
		}
		t.Logf("smoke test passed (uid %s, certs loaded, controller setup, dialed the API server)", u)
	})
}
