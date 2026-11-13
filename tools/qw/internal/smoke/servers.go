package smoke

import (
	"bytes"
	"compress/bzip2"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
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

// busyboxImage is our busybox, pinned, for side containers that write or read a volume.
const busyboxImage = "ghcr.io/quenchworks/images/busybox@sha256:041302c8da3a2e182ecc2eac3c56c4f47a13c03cc284d255489c0be741893b64"

func init() {
	register("prometheus-config-reloader", func(t *T) {
		t.User("1001")
		t.WantVersion(t.Run("--version"))
		vol, name := t.Name("vol"), t.Name("")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		// the input config uses $(POD_NAME), which the reloader must substitute
		c := exec.Command("docker", "run", "--rm", "-i", "-u", "0", "-v", vol+":/c", "--entrypoint", "/bin/sh", busyboxImage, "-c", "cat > /c/in.yaml && chown 1001 /c")
		c.Stdin = strings.NewReader("global:\n  external_labels:\n    replica: $(POD_NAME)\n")
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("seed config: %v %s", err, out)
		}
		t.Start(name, []string{"--read-only", "-e", "POD_NAME=quench-smoke-0", "-v", vol + ":/c", "-p", "127.0.0.1:18085:8080"},
			"--listen-address=:8080", "--config-file=/c/in.yaml", "--config-envsubst-file=/c/out.yaml", "--reload-url=http://127.0.0.1:9/-/reload")
		t.WaitHTTP("http://127.0.0.1:18085/healthz", 30, name)
		var out string
		for i := 0; i < 20; i++ {
			if out, _ = docker("run", "--rm", "-v", vol+":/c", "--entrypoint", "/bin/cat", busyboxImage, "/c/out.yaml"); strings.Contains(out, "quench-smoke-0") {
				break
			}
			time.Sleep(time.Second)
		}
		t.Contains(out, "replica: quench-smoke-0", "rendered config")
		t.Logf("  rendered config carries replica: quench-smoke-0; /healthz ok\nsmoke test passed")
	})
	register("trivy-scanner", func(t *T) {
		t.User("1001")
		ver := strings.SplitN(t.RunAny("--version"), "\n", 2)[0]
		t.Logf("%s", ver)
		if t.Want != "" {
			t.Contains(ver, "Version: "+t.Want, "version")
		}
		// the shape of command Trivy Operator's scan Jobs run: /bin/sh -c, trivy, bzip2, base64,
		// on a read-only root. A random AWS-shaped key is planted and must be found.
		key, secret := "AKIA"+randFrom("ABCDEFGHIJKLMNOPQRSTUVWXYZ234567", 16), randFrom("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789/+", 40)
		out, err := t.RunFlags([]string{"--read-only", "--tmpfs", "/tmp", "-e", "KEYID=" + key, "-e", "SECRET=" + secret, "--entrypoint", "/bin/sh"}, "-c", `
mkdir -p /tmp/scan /tmp/fs/etc && printf "aws_access_key_id = %s\naws_secret_access_key = %s\n" "$KEYID" "$SECRET" > /tmp/fs/etc/creds
trivy fs --scanners secret --format json --output /tmp/scan/result.json /tmp/fs 2>/tmp/scan/result.json.log
rc=$?; if [ $rc -eq 1 ]; then cat /tmp/scan/result.json.log; else bzip2 -c /tmp/scan/result.json | base64; fi; exit $rc`)
		if err != nil {
			t.Fatalf("scan job: %v %s", err, out)
		}
		raw, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(out), ""))
		if err != nil {
			t.Fatalf("scan output is not base64: %v", err)
		}
		var rep struct {
			Results []struct{ Secrets []any } `json:"Results"`
		}
		if err := json.NewDecoder(bzip2.NewReader(bytes.NewReader(raw))).Decode(&rep); err != nil {
			t.Fatalf("scan output is not bzip2 JSON: %v", err)
		}
		n := 0
		for _, r := range rep.Results {
			n += len(r.Secrets)
		}
		t.Logf("secrets found: %d", n)
		if n == 0 {
			t.Fatalf("the planted secret was not found")
		}
		t.Logf("smoke test passed")
	})
	register("woodpecker", func(t *T) {
		for _, b := range []string{"woodpecker-server", "woodpecker-agent", "woodpecker-cli"} {
			t.Match(t.RunEntry("/usr/bin/"+b, "--version"), `[0-9]+\.[0-9]+\.[0-9]+`, b+" version not stamped")
		}
		u := t.User("1001")
		t.Logf("smoke test passed (nonroot user: %s)", u)
	})
	register("ztunnel", func(t *T) {
		v := t.RunAny("version")
		t.Logf("ztunnel version: %s", v)
		if t.Want != "" {
			t.Contains(v, `Version:"`+t.Want+`"`, "version")
		}
		name := t.Name("")
		t.Start(name, []string{"--cap-add", "NET_ADMIN", "-p", "127.0.0.1:15020:15020", "-p", "127.0.0.1:15021:15021",
			"-e", "XDS_ADDRESS=", "-e", "PROXY_MODE=dedicated", "-e", "PROXY_WORKLOAD_INFO=default/local/default"}, "proxy")
		var body string
		for i := 0; i < 30; i++ {
			if _, body = Get("http://127.0.0.1:15020/metrics"); regexp.MustCompile(`(?m)^# TYPE`).MatchString(body) {
				break
			}
			if i == 29 {
				t.Fatalf("stats server (:15020/metrics) did not answer\n%s", t.Logs(name))
			}
			time.Sleep(time.Second)
		}
		// no XDS server: readiness answers, ready or not
		if code, _ := Get("http://127.0.0.1:15021/healthz/ready"); code < 200 || code >= 600 || (code >= 300 && code < 500) {
			t.Fatalf("readiness server did not answer (%d)", code)
		}
		t.Logf("smoke test passed")
	})
	register("pushgateway", func(t *T) {
		t.User("1001")
		if t.Want != "" {
			t.Contains(t.Run("--version"), "version "+t.Want, "version")
		}
		name := t.Name("")
		t.Start(name, []string{"--read-only", "-p", "127.0.0.1:19091:9091"})
		const b = "http://127.0.0.1:19091"
		t.WaitHTTP(b+"/-/ready", 30, name)
		t.WaitHTTP(b+"/-/healthy", 1, name)
		if code, body := Do("POST", b+"/metrics/job/quench_smoke", "quench_smoke_value 42\n"); code != 200 {
			t.Fatalf("push: %d %s", code, body)
		}
		_, m := Get(b + "/metrics")
		t.Match(m, `(?m)^quench_smoke_value\{.*job="quench_smoke".*\} 42$`, "pushed metric")
		if code, body := Do("DELETE", b+"/metrics/job/quench_smoke", ""); code != 202 && code != 200 {
			t.Fatalf("delete: %d %s", code, body)
		}
		if _, m = Get(b + "/metrics"); regexp.MustCompile(`(?m)^quench_smoke_value`).MatchString(m) {
			t.Fatalf("deleted group still exposed")
		}
		noCrash(t, t.Logs(name))
		t.Logf("smoke test passed")
	})
	register("filebrowser-quantum", func(t *T) {
		out := t.Run("version")
		t.Logf("%s", out)
		t.Match(out, `v?[0-9]+\.[0-9]+\.[0-9]+`, "version not stamped")
		t.WantVersion(out)
		t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"-p", "18081:8080", "-e", "FILEBROWSER_ADMIN_PASSWORD=smoke-test-pw", "--tmpfs", "/tmp"})
		ok := false
		for i := 0; i < 60 && !ok; i++ {
			h, _ := Get("http://127.0.0.1:18081/health")
			r, _ := Get("http://127.0.0.1:18081/")
			ok = h == 200 && (r == 200 || r == 302)
			if !ok {
				time.Sleep(time.Second)
			}
		}
		if !ok {
			t.Fatalf("server not healthy\n%s", t.Logs(name))
		}
		_, page := Get("http://127.0.0.1:18081/")
		t.Match(page, `(?i)<div id="app"|<script`, "UI served")
		t.Logf("smoke test passed")
	})
	register("rabbitmq", func(t *T) {
		name := t.Name("")
		t.Start(name, nil)
		for i := 0; ; i++ {
			if _, err := docker("exec", name, "rabbitmq-diagnostics", "-q", "check_running"); err == nil {
				t.Logf("node fully booted after ~%ds", i*2)
				break
			}
			if i == 60 {
				t.Fatalf("rabbitmq did not become ready\n%s", t.Logs(name))
			}
			time.Sleep(2 * time.Second)
		}
		u := t.User("1001")
		v, _ := docker("exec", name, "rabbitmqctl", "--version")
		t.Logf("version: %s\nsmoke test passed (nonroot user: %s)", v, u)
	})
	register("vcluster", func(t *T) {
		t.User("1001")
		ver := t.RunEntry("/vcluster", "version")
		t.Logf("vcluster: %s", ver)
		if ver == "" || ver == "dev" || strings.Contains(ver, "0.0.0") {
			t.Fatalf("version not stamped: '%s'", ver)
		}
		if t.Want != "" && ver != t.Want {
			t.Fatalf("expected %s", t.Want)
		}
		t.RunEntry("/vcluster", "start", "--help")
		for _, b := range []string{"kube-apiserver", "kube-controller-manager"} {
			if kv := t.RunEntry("/binaries/"+b, "--version"); kv != "Kubernetes v1.36.4" {
				t.Fatalf("/binaries/%s reports '%s'", b, kv)
			}
		}
		t.RunEntry("/binaries/kine", "--version")
		t.Logf("smoke test passed")
	})
	register("flink", func(t *T) {
		if t.Want == "" {
			t.Fatalf("usage: qw smoke flink <image-ref> <version>")
		}
		t.User("1001")
		net, jm, tm := t.Name("net"), t.Name("jm"), t.Name("tm")
		t.Docker("network", "create", net)
		t.Cleanup(func() { docker("network", "rm", net) })
		t.Start(jm, []string{"--network", net, "--hostname", "jobmanager", "--read-only", "--tmpfs", "/tmp", "-e", "JOB_MANAGER_RPC_ADDRESS=jobmanager", "-p", "127.0.0.1:18081:8081"}, "jobmanager")
		t.Start(tm, []string{"--network", net, "--read-only", "--tmpfs", "/tmp", "-e", "JOB_MANAGER_RPC_ADDRESS=jobmanager", "-e", "TASK_MANAGER_NUMBER_OF_TASK_SLOTS=2"}, "taskmanager")
		var out string
		for i := 0; i < 90; i++ {
			if _, out = Get("http://127.0.0.1:18081/overview"); strings.Contains(out, `"taskmanagers":1`) {
				break
			}
			if i == 89 {
				t.Fatalf("no taskmanager registered: %s\n%s\n%s", out, t.Logs(jm), t.Logs(tm))
			}
			time.Sleep(time.Second)
		}
		t.Logf("%s", out)
		t.Contains(out, `"flink-version":"`+t.Want+`"`, "version")
		t.Contains(out, `"slots-available":2`, "free slots")
		t.Logf("smoke test passed")
	})
	register("fulcio", func(t *T) {
		t.User("1001")
		t.Match(t.RunEntry("/usr/bin/fulcio", "version"), `GitVersion: *v`+regexp.QuoteMeta(t.Want), "version")
		name := t.Name("")
		t.Start(name, []string{"-p", "127.0.0.1:18080:8080"}, "--ca", "ephemeralca")
		bundle := t.WaitHTTP("http://127.0.0.1:18080/api/v2/trustBundle", 30, name)
		t.Contains(bundle, "BEGIN CERTIFICATE", "trust bundle")
		_, conf := Get("http://127.0.0.1:18080/api/v2/configuration")
		t.Contains(conf, `"issuers"`, "issuer configuration")
		t.Logf("  trust bundle and issuer configuration served\nsmoke test passed")
	})
}
