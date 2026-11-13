package smoke

import (
	"crypto/tls"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// getH does a GET and returns status, headers and body (code 0 on a transport error).
func getH(c *http.Client, url string) (int, http.Header, string) {
	r, err := c.Get(url)
	if err != nil {
		return 0, nil, err.Error()
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return r.StatusCode, r.Header, string(b)
}

// jsRuntime checks a Node-style runtime image (yarn, bun): major version, read-only root, uid 1001.
func jsRuntime(t *T) {
	if t.Want == "" {
		t.Fatalf("usage: qw smoke %s <image-ref> <major>", t.App)
	}
	ro := []string{"--read-only", "--tmpfs", "/tmp"}
	v, _ := t.RunFlags(ro, "--version")
	if !strings.HasPrefix(v, strings.SplitN(t.Want, ".", 2)[0]+".") {
		t.Fatalf("expected '%s.*', got '%s'", t.Want, v)
	}
	t.User("1001")
	t.Logf("%s %s", t.App, v)
}

func init() {
	register("nginx-gateway-fabric", func(t *T) {
		out, _ := t.RunFlags([]string{"-e", "POD_NAMESPACE=default", "-e", "POD_NAME=t", "-e", "POD_UID=t", "-e", "INSTANCE_NAME=t", "-e", "IMAGE_NAME=t"},
			"controller", "--gateway-ctlr-name=gateway.nginx.org/nginx-gateway-controller", "--gatewayclass=nginx")
		t.Contains(out, "Starting the NGINX Gateway Fabric control plane", "startup log")
		m := regexp.MustCompile(`"version":"([^"]*)"`).FindStringSubmatch(out)
		if m == nil || m[1] == "" || m[1] == "0.0.0" || regexp.MustCompile(`dev|unknown`).MatchString(m[1]) {
			t.Fatalf("version not stamped: %v", m)
		}
		if t.Want != "" && m[1] != t.Want {
			t.Fatalf("version '%s', expected '%s'", m[1], t.Want)
		}
		if strings.Contains(strings.ToLower(out), "telemetry report period") {
			t.Fatalf("binary is missing the telemetry ldflags")
		}
		t.Match(out, `(?i)kubeconfig|unable to|connection refused|cannot|no such host|KUBERNETES_SERVICE_HOST`, "reported the missing cluster config")
		for _, sub := range []string{"controller", "generate-certs", "initialize"} {
			t.Run(sub, "--help")
		}
		t.Logf("smoke test passed (version %s, nonroot user: %s)", m[1], t.User("101"))
	})
	register("percona-xtradb-cluster-operator", func(t *T) {
		u := t.User("1001")
		w := t.TempDir()
		for _, d := range []string{"mysql", "percona"} {
			os.Mkdir(filepath.Join(w, d), 0o777)
			os.Chmod(filepath.Join(w, d), 0o777)
		}
		if out, err := t.RunFlags([]string{"--read-only", "-v", w + "/mysql:/var/lib/mysql", "--entrypoint", "/pxc-init-entrypoint.sh"}); err != nil {
			t.Fatalf("pxc-init-entrypoint.sh failed:\n%s", out)
		}
		for _, f := range []string{"pxc-entrypoint.sh", "liveness-check.sh", "readiness-check.sh", "peer-list", "get-pxc-state", "mysql-state-monitor"} {
			if st, err := os.Stat(filepath.Join(w, "mysql", f)); err != nil || st.Mode()&0o111 == 0 {
				t.Fatalf("pxc init did not install %s", f)
			}
		}
		if out, err := t.RunFlags([]string{"--read-only", "-v", w + "/percona:/opt/percona", "--entrypoint", "/haproxy-init-entrypoint.sh"}); err != nil {
			t.Fatalf("haproxy-init-entrypoint.sh failed:\n%s", out)
		}
		for _, f := range []string{"haproxy-entrypoint.sh", "peer-list"} {
			if st, err := os.Stat(filepath.Join(w, "percona", f)); err != nil || st.Mode()&0o111 == 0 {
				t.Fatalf("haproxy init did not install %s", f)
			}
		}
		kc := t.DeadKubeconfig()
		out, _ := t.RunFlags([]string{"--read-only", "-v", kc + ":/kubeconfig:ro", "-e", "KUBECONFIG=/kubeconfig"})
		t.Contains(out, "unable to define server version", "manager reached the version lookup")
		t.Logf("smoke test passed (init scripts, manager start; nonroot user: %s)", u)
	})
	register("skipper", func(t *T) {
		u := t.User("1001")
		if v := t.Run("-version"); t.Want != "" {
			t.Contains(v, "Skipper version v"+t.Want+" ", "version")
		}
		net_, b, f := t.Name("net"), t.Name("b"), t.Name("")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Start(b, []string{"--network", net_, "--read-only"}, "-inline-routes", `r: * -> inlineContent("hello from backend") -> <shunt>`)
		t.Start(f, []string{"--network", net_, "--read-only", "-p", "127.0.0.1:19090:9090", "-p", "127.0.0.1:19911:9911"},
			"-inline-routes", `r: Path("/app") -> setPath("/") -> setResponseHeader("X-Quench", "ok") -> "http://`+b+`:9090"`)
		var h http.Header
		for i := 0; ; i++ {
			var body string
			if _, h, body = getH(httpc, "http://127.0.0.1:19090/app"); strings.Contains(body, "hello from backend") {
				break
			}
			if i == 30 {
				t.Fatalf("proxy route never answered\n%s", t.Logs(f))
			}
			time.Sleep(time.Second)
		}
		if h.Get("X-Quench") != "ok" {
			t.Fatalf("filter header missing: %v", h)
		}
		if c, _ := Get("http://127.0.0.1:19090/nope"); c != 404 {
			t.Fatalf("unrouted path got HTTP %d", c)
		}
		_, m := Get("http://127.0.0.1:19911/metrics")
		t.Match(m, `skipper|go_goroutines`, "metrics")
		t.RunEntryAny("/usr/bin/eskip", "check", "-routes", "a: * -> <shunt>")
		t.Logf("smoke test passed: proxy route, filter, 404, metrics, user %s", u)
	})
	register("tomcat", func(t *T) {
		name := t.Name("")
		t.Start(name, []string{"-p", "8080"})
		url := "http://localhost:" + t.hostPort(name, "8080/tcp") + "/"
		var c int
		var h http.Header
		var body string
		for i := 0; ; i++ {
			if c, h, body = getH(httpc, url); c != 0 {
				break
			}
			if i == 60 {
				t.Fatalf("tomcat did not answer on %s\n%s", url, t.Logs(name))
			}
			time.Sleep(time.Second)
		}
		if c != 200 && c != 404 {
			t.Fatalf("unexpected HTTP status %d", c)
		}
		if !regexp.MustCompile(`(?i)Coyote|Tomcat`).MatchString(h.Get("Server")) && !strings.Contains(strings.ToLower(body), "apache tomcat") {
			t.Fatalf("response did not identify as Tomcat")
		}
		t.Logf("smoke test passed (HTTP %d, nonroot user: %s)", c, t.User("1001"))
	})
	register("trickster", func(t *T) {
		u := t.User("1001")
		if v := t.RunAny("-version"); t.Want != "" {
			t.Contains(v, t.Want, "version")
		}
		net_, origin, name := t.Name("net"), t.Name("origin"), t.Name("")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Cleanup(func() { docker("rm", "-f", origin) })
		t.Docker("run", "-d", "--name", origin, "--network", net_, "--entrypoint", "/usr/bin/caddy", "ghcr.io/quenchworks/images/caddy:2.11.9",
			"respond", "--listen", ":8000", "-H", "Cache-Control: max-age=300", "-b", "quench-origin")
		t.Start(name, []string{"--network", net_, "--read-only", "-p", "127.0.0.1:18480:8480", "-p", "127.0.0.1:18481:8481"}, "-origin-url", "http://"+origin+":8000", "-provider", "rpc")
		var h http.Header
		var body string
		for i := 0; ; i++ {
			if _, h, body = getH(httpc, "http://127.0.0.1:18480/smoke"); h.Get("X-Trickster-Result") != "" {
				break
			}
			if i == 30 {
				t.Fatalf("trickster never answered\n%s", t.Logs(name))
			}
			time.Sleep(time.Second)
		}
		if body != "quench-origin" {
			t.Fatalf("origin body not proxied: %s", body)
		}
		_, h2, _ := getH(httpc, "http://127.0.0.1:18480/smoke")
		r2 := h2.Get("X-Trickster-Result")
		if !regexp.MustCompile(`(?i)status=(hit|phit)`).MatchString(r2) {
			t.Fatalf("second request was not served from cache: %s", r2)
		}
		_, m := Get("http://127.0.0.1:18481/metrics")
		t.Match(m, `(?m)^trickster_`, "metrics")
		if regexp.MustCompile(`panic|read-only file system`).MatchString(t.Logs(name)) {
			t.Fatalf("crashed or wrote to the root\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed: proxied origin, cache hit on repeat (%s), metrics, user %s", r2, u)
	})
	register("wiremock", func(t *T) {
		u := t.User("1001")
		w := t.TempDir()
		os.Mkdir(filepath.Join(w, "mappings"), 0o755)
		os.WriteFile(filepath.Join(w, "mappings/hello.json"), []byte(`{"request":{"method":"GET","urlPath":"/hello"},
 "response":{"status":200,"transformers":["response-template"],
   "body":"hi {{request.query.name}} {{numberFormat 1234.5 '#,##0.00'}} {{math 2 '+' 3}}"}}`), 0o644)
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp", "-v", w + "/mappings:/home/wiremock/mappings:ro", "-p", "127.0.0.1:18080:8080", "-p", "127.0.0.1:18443:8443"}, "--https-port", "8443")
		b := "http://127.0.0.1:18080"
		for i := 0; ; i++ {
			if _, h := Get(b + "/__admin/health"); regexp.MustCompile(`"status" *: *"healthy"`).MatchString(h) {
				if t.Want != "" {
					t.Match(h, `"version" *: *"`+regexp.QuoteMeta(t.Want)+`"`, "version")
				}
				break
			}
			if i == 60 {
				t.Fatalf("wiremock never reported healthy\n%s", t.Logs(name))
			}
			time.Sleep(time.Second)
		}
		if _, got := Get(b + "/hello?name=quench"); got != "hi quench 1,234.50 5" {
			t.Fatalf("templating: %s", got)
		}
		Post(b+"/__admin/mappings", "application/json", `{"request":{"method":"GET","urlPathPattern":"/via/.*"},"response":{"proxyBaseUrl":"http://127.0.0.1:8080","proxyUrlPrefixToRemove":"/via"}}`)
		if _, got := Get(b + "/via/hello?name=proxied"); got != "hi proxied 1,234.50 5" {
			t.Fatalf("proxy: %s", got)
		}
		h2 := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{ForceAttemptHTTP2: true, TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
		r, err := h2.Get("https://127.0.0.1:18443/__admin/health")
		if err != nil || r.StatusCode != 200 || r.ProtoMajor != 2 {
			t.Fatalf("HTTPS/2 health: %v %v", err, r)
		}
		r.Body.Close()
		_, j := Get(b + "/__admin/requests")
		if n := len(regexp.MustCompile(`"url" *: *"/[^"]*hello`).FindAllString(j, -1)); n < 2 {
			t.Fatalf("journal recorded %d hello calls", n)
		}
		if regexp.MustCompile(`Exception|ERROR`).MatchString(t.Logs(name)) {
			t.Fatalf("wiremock logged errors\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed (uid %s, templating, proxy, HTTPS/2, journal)", u)
	})
	register("yarn", func(t *T) {
		jsRuntime(t)
		ro := []string{"--read-only", "--tmpfs", "/tmp"}
		nv, _ := t.RunFlags(append(ro, "--entrypoint", "/usr/bin/node"), "-v")
		t.Match(nv, `^v[0-9]`, "node -v")
		sh, _ := t.RunFlags(append(ro, "--entrypoint", "/bin/sh"), "-c", "echo sh-ok")
		t.Contains(sh, "sh-ok", "/bin/sh backs the yarn launcher")
		if uid, _ := t.RunFlags(append(ro, "--entrypoint", "/usr/bin/node"), "-e", "process.stdout.write(String(process.getuid()))"); uid != "1001" {
			t.Fatalf("expected uid 1001, got '%s'", uid)
		}
		if _, err := t.RunFlags(append(ro, "--entrypoint", "/usr/bin/node"), "-e", `require("fs").writeFileSync("/should-fail","x")`); err == nil {
			t.Fatalf("rootfs was writable, expected read-only")
		}
		t.Logf("smoke test passed (node %s, read-only rootfs)", nv)
	})
	register("bun", func(t *T) {
		jsRuntime(t)
		ro := []string{"--read-only", "--tmpfs", "/tmp"}
		if out, _ := t.RunFlags(ro, "-e", `console.log("quench-" + "bun")`); out != "quench-bun" {
			t.Fatalf("eval output wrong: '%s'", out)
		}
		if uid, _ := t.RunFlags(ro, "-e", "console.log(process.getuid())"); uid != "1001" {
			t.Fatalf("expected uid 1001, got '%s'", uid)
		}
		w := t.TempDir()
		os.WriteFile(filepath.Join(w, "h.ts"), []byte("const msg: string = \"quench-bun-ts\";\nconsole.log(msg);\n"), 0o644)
		if out, _ := t.RunFlags(append(ro, "-v", w+":/work:ro", "-w", "/work"), "run", "h.ts"); out != "quench-bun-ts" {
			t.Fatalf("TS run output wrong: '%s'", out)
		}
		if out, _ := t.RunFlags(ro, "-e", `try { require("fs").writeFileSync("/should-fail","x"); console.log("WROTE"); } catch(e){ console.log(e.code); }`); out != "EROFS" {
			t.Fatalf("rootfs was writable (got '%s')", out)
		}
		t.Logf("smoke test passed (read-only rootfs, TS run)")
	})
	register("apache-pinot", func(t *T) {
		u := t.User("1001")
		if t.Want != "" {
			t.Contains(t.RunEntry("/bin/sh", "-c", "ls /opt/pinot/lib/"), "pinot-all-"+t.Want+"-jar-with-dependencies.jar", "pinot jar version")
		}
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,exec,size=3g", "--memory", "5g", "-e", "JAVA_OPTS=-Xms512m -Xmx2g", "-e", "HOME=/tmp",
			"-p", "127.0.0.1::8000", "--entrypoint", "/bin/sh"},
			"-c", "mkdir -p /tmp/w && cp -r /opt/pinot/examples /tmp/w/ && cd /tmp/w && exec /opt/pinot/bin/pinot-admin.sh QuickStart -type batch -dataDir /tmp/qs")
		url := "http://127.0.0.1:" + t.hostPort(name, "8000/tcp") + "/query/sql"
		rows := regexp.MustCompile(`"rows":\[\[([1-9][0-9]*)`)
		var out string
		for i := 0; ; i++ {
			_, out = Post(url, "application/json", `{"sql":"select count(*) from airlineStats"}`)
			if rows.MatchString(out) {
				break
			}
			if s := t.Docker("inspect", "-f", "{{.State.Status}}", name); s != "running" {
				t.Fatalf("container died:\n%s", t.Logs(name))
			}
			if i == 240 {
				t.Fatalf("no rows from the broker: %.300s\n%s", out, t.Logs(name))
			}
			time.Sleep(2 * time.Second)
		}
		if m := regexp.MustCompile(`(?i)read-only file system|NoClassDefFoundError|NoSuchMethodError|OutOfMemoryError`).FindString(t.Logs(name)); m != "" {
			t.Fatalf("errors in the logs: %s", m)
		}
		t.Logf("smoke test passed (nonroot %s, QuickStart batch answered %s rows)", u, rows.FindStringSubmatch(out)[1])
	})
	register("centrifugo", func(t *T) {
		v := t.RunAny("version")
		t.Match(v, `(?i)v[0-9]+\.[0-9]+\.[0-9]+`, "version stamped")
		if strings.Contains(v, "v0.0.0") {
			t.Fatalf("version is default 0.0.0")
		}
		u := t.User("1001")
		t.httpUp([]string{"--read-only", "-e", "CENTRIFUGO_HEALTH_ENABLED=true", "-e", "CENTRIFUGO_CLIENT_ALLOWED_ORIGINS=*", "-p", "127.0.0.1:18000:8000"}, "http://127.0.0.1:18000/health", 20)
		t.Logf("smoke test passed (nonroot user: %s, health OK on read-only rootfs)", u)
	})
}
