package smoke

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// fluxDeadAPI: a Flux controller given a kubeconfig whose API server never answers must
// start, dial that server, and neither panic nor write to its read-only root.
func fluxDeadAPI(flags []string, args ...string) func(*T) {
	return func(t *T) {
		t.User("1001")
		kc := t.DeadKubeconfig()
		f := append([]string{"--read-only", "--tmpfs", "/tmp"}, flags...)
		f = append(f, "-e", "KUBECONFIG=/etc/kubeconfig", "-v", kc+":/etc/kubeconfig:ro")
		out, _ := t.RunAnyFor(25*time.Second, f, args...)
		t.Contains(out, "127.0.0.1:1", "dialed the configured API server")
		noCrash(t, out)
		t.Logf("  started, dialed 127.0.0.1:1, no panic, nothing written to the root\nsmoke test passed")
	}
}

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
	register("flux-helm-controller", fluxDeadAPI(nil))
	register("flux-image-automation-controller", fluxDeadAPI(nil))
	register("flux-notification-controller", fluxDeadAPI(nil))
	register("flux-image-reflector-controller", fluxDeadAPI(nil, "--storage-path=/tmp/data"))
	register("flux-source-controller", fluxDeadAPI(nil, "--storage-path=/tmp/data"))
	// the artifact directory must already exist: a tmpfs here, an emptyDir in the chart
	register("flux-source-watcher", fluxDeadAPI([]string{"--tmpfs", "/tmp/data:uid=1001,gid=1001"}, "--storage-path=/tmp/data"))
	register("dive", func(t *T) {
		t.User("1001")
		ver := t.Run("--version")
		t.Logf("%s", ver)
		if t.Want != "" && ver != "dive "+t.Want {
			t.Fatalf("expected dive %s", t.Want)
		}
		// dive analyses its own image from a docker-archive in --ci mode (no Docker socket)
		tar := filepath.Join(t.TempDir(), "image.tar")
		t.Docker("save", t.Image, "-o", tar)
		os.Chmod(tar, 0o644)
		name := t.Name("")
		t.Cleanup(func() { docker("rm", "-f", name) })
		t.Docker("create", "--name", name, "-e", "CI=true", t.Image, "--ci", "--source", "docker-archive", "/image.tar")
		t.Docker("cp", tar, name+":/image.tar")
		out, _ := docker("start", "-a", name)
		t.Contains(out, "efficiency:", "dive analysis")
		t.Contains(out, "Result:PASS", "dive --ci on its own image")
		t.Logf("smoke test passed")
	})
	register("graylog", func(t *T) {
		t.User("1001")
		t.Logf("%s", t.RunEntry("java", "-version"))
		if _, err := t.RunFlags([]string{"--entrypoint", "sh"}, "-c", "test -f /opt/graylog/graylog.jar"); err != nil {
			t.RunEntry("/usr/bin/busybox", "sh", "-c", "test -f /opt/graylog/graylog.jar")
		}
		// the entrypoint's non-server passthrough: no Mongo or OpenSearch needed
		t.Match(t.Run("version"), `(?im)^Graylog [0-9]+\.[0-9]+\.[0-9]+`, "version")
		t.Logf("smoke test passed")
	})
	register("hubble-relay", func(t *T) {
		t.User("1001")
		t.WantVersion(t.RunEntry("/usr/bin/hubble-relay", "version"))
		// no Cilium peer: it must open the Observer and health ports and keep retrying the peer
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp", "-p", "127.0.0.1:14245:4245", "-p", "127.0.0.1:14222:4222"},
			"--disable-server-tls", "--disable-client-tls", "--peer-service=127.0.0.1:1")
		t.WaitTCP(30, name, "127.0.0.1:14245", "127.0.0.1:14222")
		time.Sleep(3 * time.Second)
		if t.Docker("inspect", name, "--format", "{{.State.Running}}") != "true" {
			t.Fatalf("relay exited:\n%s", t.Logs(name))
		}
		t.Contains(t.Logs(name), "Starting gRPC server", "relay log")
		t.Logf("smoke test passed")
	})
	register("keda", func(t *T) {
		t.ContainsFold(t.RunAny("--help"), "--leader-elect", "operator --help")
		t.Match(t.RunEntryAny("/usr/bin/keda-adapter", "--help"), `(?i)usage|--secure-port|--help`, "adapter --help")
		u := t.User("1001")
		t.Logf("smoke test passed (nonroot user: %s)", u)
	})
	register("kyverno", func(t *T) {
		out := t.RunEntry("/usr/bin/kubectl-kyverno", "version")
		t.Logf("%s", out)
		t.Match(out, `(?i)Version:\s*v?[0-9]+\.[0-9]+\.[0-9]+`, "version not stamped")
		t.RunEntry("/usr/bin/kyverno", "--help")
		u := t.User("1001")
		t.Logf("smoke test passed (nonroot user: %s)", u)
	})
}
