package smoke

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// noCrash fails if out shows a panic or a write to the read-only root.
func noCrash(t *T, out string) {
	if regexp.MustCompile(`panic:|read-only file system`).MatchString(out) {
		t.Fatalf("crashed or wrote to the root:\n%s", out)
	}
}

func init() {
	register("oauth2-proxy", func(t *T) {
		f := strings.Fields(t.Run("--version"))
		if len(f) < 2 {
			t.Fatalf("no version")
		}
		ver := f[1]
		t.Logf("reported version: %s", ver)
		if ver == "0.0.0" || strings.Contains(ver, "dev") || strings.Contains(ver, "undefined") {
			t.Fatalf("version not stamped: '%s'", ver)
		}
		t.Run("--help")
		u := t.User("1001")
		t.Logf("smoke test passed (nonroot user: %s)", u)
	})
	register("tekton", func(t *T) {
		// nop runs and exits with no dependencies; the servers need a cluster
		t.ContainsFold(t.RunEntry("/usr/bin/nop"), "Exiting", "nop")
		t.Contains(t.Docker("inspect", t.Image, "--format", "{{json .Config.Entrypoint}}"), "/usr/bin/controller", "entrypoint")
		u := t.User("1001")
		t.Logf("smoke test passed (nonroot user: %s)", u)
	})
	register("openssl", func(t *T) {
		ver := t.Run("version")
		t.Logf("  %s", ver)
		if t.Want != "" {
			t.Contains(ver, "OpenSSL "+t.Want+" ", "version")
		}
		// a CA, a leaf, and the PKCS12 keystore the chart helper builds, read back
		out := t.RunEntry("/bin/sh", "-c", `set -eu; cd /tmp
openssl req -x509 -newkey rsa:2048 -nodes -keyout ca.key -out ca.crt -subj /CN=ca -days 1 2>/dev/null
openssl req -newkey rsa:2048 -nodes -keyout tls.key -out tls.csr -subj /CN=localhost 2>/dev/null
openssl x509 -req -in tls.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out tls.crt -days 1 2>/dev/null
openssl pkcs12 -export -inkey tls.key -in tls.crt -certfile ca.crt -name tls -passout pass:changeit -out ks.p12
openssl pkcs12 -in ks.p12 -passin pass:changeit -nokeys | grep -c "BEGIN CERTIFICATE"`)
		if out != "2" {
			t.Fatalf("PKCS12 round trip failed: %s", out)
		}
		t.Logf("  PKCS12 export + read back: leaf and CA present")
		u := t.User("1001")
		t.Logf("smoke test passed (nonroot user: %s)", u)
	})
	register("prometheus-nats-exporter", func(t *T) {
		t.User("1001")
		ver, _ := t.RunFlags([]string{"--read-only"}, "-version")
		t.WantVersion(ver)
		// scrape the QuenchWorks nats image (JetStream on, monitoring port) with Argo Events' flags
		net, nats := t.Name("net"), t.Name("nats")
		t.Docker("network", "create", net)
		t.Cleanup(func() { docker("network", "rm", net) })
		t.Cleanup(func() { docker("rm", "-f", nats) })
		t.Docker("run", "-d", "--name", nats, "--network", net, "--tmpfs", "/tmp", "ghcr.io/quenchworks/images/nats:2.15.0", "-js", "-sd", "/tmp/js", "-m", "8222")
		name := t.Name("")
		t.Start(name, []string{"--network", net, "--read-only", "-p", "127.0.0.1:17777:7777"},
			"-connz", "-routez", "-subz", "-varz", "-prefix=nats", "-use_internal_server_id", "-jsz=all", "http://"+nats+":8222")
		var m string
		for i := 0; i < 30; i++ {
			if _, m = Get("http://127.0.0.1:17777/metrics"); regexp.MustCompile(`(?m)^nats_varz_`).MatchString(m) {
				break
			}
			time.Sleep(time.Second)
		}
		t.Match(m, `(?m)^nats_varz_connections`, "varz connections")
		t.Match(m, `(?m)^nats_.*jetstream|^nats_jetstream_|_jsz_`, "JetStream series")
		t.Logf("smoke test passed")
	})
	register("rqlite", func(t *T) {
		t.Match(t.RunEntry("/usr/bin/rqlited", "-version"), `[0-9]+\.[0-9]+\.[0-9]+`, "rqlited version not stamped")
		// the CLI reads -version as bundled short flags; -v is its version flag
		t.Match(t.RunEntry("/usr/bin/rqlite", "-v"), `[0-9]+\.[0-9]+\.[0-9]+`, "rqlite CLI version not stamped")
		u := t.User("1001")
		t.Logf("smoke test passed (nonroot user: %s)", u)
	})
	register("argo-rollouts", func(t *T) {
		t.User("1001")
		kc := t.DeadKubeconfig()
		out, _ := t.RunAnyFor(60*time.Second, []string{"--read-only", "--tmpfs", "/tmp", "-v", kc + ":/etc/kubeconfig:ro"}, "--kubeconfig", "/etc/kubeconfig")
		t.Contains(out, "Argo Rollouts controller starting", "startup line")
		if t.Want != "" {
			t.Contains(out, "version=v"+t.Want, "stamped version")
		}
		t.Contains(out, "127.0.0.1:1/version", "API discovery against the configured server")
		noCrash(t, out)
		t.Logf("smoke test passed")
	})
	register("cloudnative-pg", func(t *T) {
		t.User("1001")
		t.Contains(t.Run("version"), "Version:"+t.Want, "stamped version")
		// every PostgreSQL pod starts with `manager bootstrap <dest>`, then runs the copy
		work := t.TempDir()
		if out, err := t.RunFlags([]string{"--read-only", "-v", work + ":/controller"}, "bootstrap", "/controller/manager"); err != nil {
			t.Fatalf("bootstrap: %v %s", err, out)
		}
		if fi, err := os.Stat(filepath.Join(work, "manager")); err != nil || fi.Mode()&0o111 == 0 {
			t.Fatalf("bootstrap did not copy an executable manager")
		}
		out, err := t.RunFlags([]string{"--read-only", "-v", work + ":/controller:ro", "--entrypoint", "/controller/manager"}, "version")
		if err != nil || !strings.Contains(out, "Version:"+t.Want) {
			t.Fatalf("the copied manager does not run: %v %s", err, out)
		}
		t.Logf("smoke test passed")
	})
	register("distribution", func(t *T) {
		name := t.Name("")
		t.Start(name, []string{"-p", "127.0.0.1:5000:5000"})
		// /v2/ is the registry API base: 200 once the server is ready
		t.WaitHTTP("http://127.0.0.1:5000/v2/", 30, name)
		u := t.User("1001")
		v, _ := docker("exec", name, "registry", "--version")
		t.Logf("version: %s\nsmoke test passed (nonroot user: %s)", v, u)
	})
	register("emissary", func(t *T) {
		t.Match(t.RunEntry("/usr/local/bin/envoy", "--version"), `/1\.37\.[0-9]+/`, "envoy 1.37")
		t.RunEntry("/opt/ambassador/emissary/bin/python", "-c", "import ambassador, ambassador_diag.diagd, orjson, requests")
		// without a cluster the entrypoint starts ambex, diagd and envoy, then stops when the watcher cannot reach an API server
		out, _ := t.RunAnyFor(60*time.Second, nil)
		ver := t.Match(out, `Started Ambassador \(Version ([^)]+)\)`, "start line")
		if t.Want != "" && ver != t.Want {
			t.Fatalf("version %s, expected %s", ver, t.Want)
		}
		t.Contains(out, `started command [\"diagd\"`, "diagd started")
		t.Contains(out, "Listening on tcp:127.0.0.1:8003", "ambex listening")
		t.Contains(out, `goroutine \"/watcher\" exited with error: Get \"http://localhost:8080/api`, "watcher reached for the API server")
		u := t.User("8888")
		t.Logf("smoke test passed (version %s, user %s)", ver, u)
	})
	register("flux-helm-controller", func(t *T) {
		t.User("1001")
		kc := t.DeadKubeconfig()
		out, _ := t.RunAnyFor(25*time.Second, []string{"--read-only", "--tmpfs", "/tmp", "-e", "KUBECONFIG=/etc/kubeconfig", "-v", kc + ":/etc/kubeconfig:ro"})
		t.Contains(out, "127.0.0.1:1", "dialed the configured API server")
		noCrash(t, out)
		t.Logf("  started, dialed 127.0.0.1:1, no panic, nothing written to the root\nsmoke test passed")
	})
}
