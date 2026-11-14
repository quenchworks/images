package smoke

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

func init() {
	// Standalone on a read-only rootfs (conf/ and logs/ writable): the Server header carries the
	// version, the runtime's configure line has the api7 modules, the admin SPA ships, a
	// declarative route proxies to an upstream, and every plugin loads except saml-auth (this
	// image omits lua-resty-saml on purpose).
	register("apisix", func(t *T) {
		const proxy = "http://127.0.0.1:19080"
		u := t.User("1001")
		net_, up, name := t.Name("net"), t.Name("upstream"), t.Name("")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Cleanup(func() { docker("rm", "-f", up) })
		t.Docker("run", "-d", "--name", up, "--network", net_, envOr("UPSTREAM_IMAGE", "nginx:alpine"))
		t.waitFor(20, time.Second, up, "test upstream never served", func() bool {
			_, err := docker("exec", up, "sh", "-c", "wget -qO- http://127.0.0.1/")
			return err == nil
		})
		t.Start(name, []string{"--network", net_, "--read-only", "--tmpfs", "/tmp:rw,mode=1777",
			"--tmpfs", "/usr/local/apisix/conf:rw,mode=0755,uid=1001,gid=1001", "--tmpfs", "/usr/local/apisix/logs:rw,mode=0755,uid=1001,gid=1001",
			"-p", "127.0.0.1:19080:9080"})
		var server string
		t.waitFor(90, time.Second, name, "APISIX never answered on the proxy port", func() bool {
			resp, err := http.Get(proxy + "/")
			if err != nil {
				return false
			}
			resp.Body.Close()
			server = resp.Header.Get("Server")
			return true
		})
		if !strings.HasPrefix(server, "APISIX/") || (t.Want != "" && server != "APISIX/"+t.Want) {
			t.Fatalf("Server header %q, expected APISIX/%s", server, t.Want)
		}
		conf, _ := docker("exec", name, "/usr/local/openresty/bin/openresty", "-V")
		for _, m := range []string{"apisix-nginx-module", "lua-resty-events", "lua-var-nginx-module", "ngx_http_ffi_client", "http_stub_status_module", "APISIX_RUNTIME_VER="} {
			t.Contains(conf, m, "openresty configure line")
		}
		t.Docker("exec", name, "test", "-f", "/usr/local/apisix/ui/index.html")

		route := exec.Command("docker", "exec", "-i", name, "sh", "-c", "cat > /usr/local/apisix/conf/apisix.yaml")
		route.Stdin = strings.NewReader("routes:\n  - uri: /echo\n    plugins:\n      proxy-rewrite:\n        uri: /\n    upstream:\n      nodes:\n        \"" + up + ":80\": 1\n      type: roundrobin\n#END\n")
		if out, err := route.CombinedOutput(); err != nil {
			t.Fatalf("writing apisix.yaml: %v\n%s", err, out)
		}
		// standalone mode only reads apisix.yaml up to its #END marker
		t.waitFor(30, time.Second, name, "request did not traverse the proxy to the upstream", func() bool {
			_, b := Get(proxy + "/echo")
			return strings.Contains(b, "Welcome to nginx")
		})

		el, _ := docker("exec", name, "cat", "/usr/local/apisix/logs/error.log")
		failed := regexp.MustCompile(`(?m)^.*failed to load plugin.*$`).FindAllString(el, -1)
		saml := slices.IndexFunc(failed, func(l string) bool { return strings.Contains(l, "saml-auth") }) >= 0
		failed = slices.DeleteFunc(failed, func(l string) bool { return strings.Contains(l, "saml-auth") })
		if !saml {
			t.Fatalf("saml-auth loaded, but this image omits lua-resty-saml: the removal regressed or the plugin left the default list")
		}
		if len(failed) > 0 {
			t.Fatalf("%d plugin(s) failed to load:\n%s", len(failed), strings.Join(failed, "\n"))
		}
		loaded := len(regexp.MustCompile(`"[a-z0-9-]+":true`).FindAllString(regexp.MustCompile(`load\(\): new plugins: \{[^}]*\}`).FindString(el), -1))
		if loaded < 100 {
			t.Fatalf("only %d plugins loaded, expected >=100: the rock tree is incomplete", loaded)
		}
		t.Match(t.Docker("exec", name, "cat", "/proc/1/status"), `(?m)^Uid:\s+1001\s`, "pid 1 uid")
		t.Logf("smoke test passed (%s: read-only rootfs, api7 modules, admin SPA, %d plugins, proxy round trip; user %s)", server, loaded, u)
	})

	// A controller has no standalone surface, so it boots against a stub API server that only
	// answers /version and discovery: the config file's probe and metrics ports serve /healthz
	// "ok", /readyz 200 and controller_runtime_ series, and the stub sees discovery for
	// apisix.apache.org/v2 and gateway.networking.k8s.io/v1. Without a kubeconfig it must exit
	// non-zero after printing its resolved config and the in-cluster error.
	register("apisix-ingress-controller", func(t *T) {
		u := t.User("1001")
		vout := t.Run("version", "--long")
		ver := t.Match(vout, `(?m)^Version: (\S+)`, "stamped version")
		t.Match(vout, `(?m)^Git SHA: ([0-9a-f]{40})$`, "stamped git revision")
		if ver == "0.0.0" || strings.Contains(ver, "dev") || (t.Want != "" && ver != t.Want) {
			t.Fatalf("version %q, expected %s", ver, t.Want)
		}

		nk := t.Name("nokube")
		t.Start(nk, nil)
		t.waitFor(30, time.Second, nk, "did not exit within 30s with no kubeconfig", func() bool {
			return t.Docker("inspect", "-f", "{{.State.Status}}", nk) == "exited"
		})
		nout := t.Logs(nk)
		if t.Docker("inspect", "-f", "{{.State.ExitCode}}", nk) == "0" {
			t.Fatalf("exit 0 with no kubeconfig\n%s", nout)
		}
		t.Contains(nout, "controller start configuration", "resolved config printed before failing")
		t.Contains(nout, "unable to load in-cluster configuration", "no-kubeconfig failure mode")

		var mu sync.Mutex
		var seen []string
		docs := map[string]string{
			"/version": `{"major":"1","minor":"31","gitVersion":"v1.31.0","platform":"linux/amd64"}`,
			"/api":     `{"kind":"APIVersions","versions":["v1"],"serverAddressByClientCIDRs":[]}`,
			"/apis":    `{"kind":"APIGroupList","apiVersion":"v1","groups":[]}`,
		}
		api := t.hostStub(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			seen = append(seen, r.URL.Path)
			mu.Unlock()
			if d, ok := docs[r.URL.Path]; ok && r.Method == "GET" {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(d))
				return
			}
			http.NotFound(w, r)
		}))
		const probe, metrics = 18691, 18692
		dir := filepath.Join(t.TempDir(), "conf")
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "kubeconfig"), []byte(`apiVersion: v1
kind: Config
clusters: [{name: stub, cluster: {server: "`+api+`"}}]
contexts: [{name: stub, context: {cluster: stub, user: stub}}]
current-context: stub
users: [{name: stub, user: {}}]
`), 0o644)
		os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("log_level: info\nprobe_addr: \":"+strconv.Itoa(probe)+"\"\nmetrics_addr: \":"+strconv.Itoa(metrics)+
			"\"\nleader_election:\n  disable: true\nprovider:\n  type: apisix\n  sync_period: 1h\n"), 0o644)
		name := t.Name("")
		p, m := strconv.Itoa(probe), strconv.Itoa(metrics)
		t.Cleanup(func() { docker("rm", "-f", name) })
		t.Docker(append(append([]string{"create", "--name", name}, hostStubFlag...), "-p", "127.0.0.1:"+p+":"+p, "-p", "127.0.0.1:"+m+":"+m,
			"-e", "KUBECONFIG=/aic-conf/kubeconfig", t.Image, "-c", "/aic-conf/config.yaml")...)
		t.Docker("cp", dir, name+":/aic-conf")
		t.Docker("start", name)
		t.waitFor(40, time.Second, name, "controller never served /healthz on the config file's probe port", func() bool {
			if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
				t.Fatalf("controller exited\n%s", t.Logs(name))
			}
			_, b := Get("http://127.0.0.1:" + p + "/healthz")
			return b == "ok"
		})
		if c, _ := Get("http://127.0.0.1:" + p + "/readyz"); c != 200 {
			t.Fatalf("/readyz answered %d\n%s", c, t.Logs(name))
		}
		_, mout := Get("http://127.0.0.1:" + m + "/metrics")
		t.Match(mout, `(?m)^controller_runtime_`, "controller_runtime_ metrics")
		mu.Lock()
		defer mu.Unlock()
		for _, want := range []string{"/apis/apisix.apache.org/v2", "/apis/gateway.networking.k8s.io/v1"} {
			if !slices.Contains(seen, want) {
				t.Fatalf("controller never probed %s; the stub saw %v", want, seen)
			}
		}
		t.Logf("smoke test passed (apisix-ingress-controller %s: no-kubeconfig failure, probes and metrics on configured ports, CRD + Gateway API discovery; user %s)", ver, u)
	})
}
