package smoke

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// hostStub serves h on all interfaces of the host, so a container started with
// hostStubFlag can reach it as http://host.docker.internal:<port>. It replaces the
// python:3.12-slim stub servers the bash tests ran.
func (t *T) hostStub(h http.Handler) string {
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("stub listen: %v", err)
	}
	go http.Serve(ln, h)
	t.Cleanup(func() { ln.Close() })
	return "http://host.docker.internal:" + strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
}

var hostStubFlag = []string{"--add-host", "host.docker.internal:host-gateway"}

func init() {
	register("homebox", func(t *T) {
		const b = "http://127.0.0.1:17745/api/v1"
		u := t.User("1001")
		vol, name := t.Name("data"), t.Name("")
		pepper := t.Name("pepper") + "-0123456789abcdefghijklmnopqrstuv"
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		start := func() {
			t.Start(name, []string{"-e", "HBOX_AUTH_API_KEY_PEPPER=" + pepper, "--read-only", "--tmpfs", "/tmp:uid=1001,gid=1001", "-v", vol + ":/data", "-p", "127.0.0.1:17745:7745"})
			t.WaitHTTP(b+"/status", 60, name)
		}
		start()
		_, sb := Get(b + "/status")
		var st struct{ Build struct{ Version string } }
		json.Unmarshal([]byte(sb), &st)
		if st.Build.Version == "" || (t.Want != "" && st.Build.Version != t.Want) {
			t.Fatalf("homebox %q, expected %s", st.Build.Version, t.Want)
		}
		_, ui := Get("http://127.0.0.1:17745/")
		t.ContainsFold(ui, "<html", "web UI")
		j := map[string]string{"Content-Type": "application/json"}
		if c, r := DoH("POST", b+"/users/register", `{"name":"Smoke","email":"smoke@quench.test","password":"smoke-pass-2026"}`, j); c/100 != 2 {
			t.Fatalf("register: %d %s", c, r)
		}
		if c, _ := DoH("POST", b+"/users/login", `{"username":"smoke@quench.test","password":"wrong-pass"}`, j); c != 401 && c != 403 {
			t.Fatalf("a wrong password got HTTP %d", c)
		}
		auth := func() map[string]string {
			_, r := DoH("POST", b+"/users/login", `{"username":"smoke@quench.test","password":"smoke-pass-2026"}`, j)
			var l struct{ Token string }
			json.Unmarshal([]byte(r), &l)
			if l.Token == "" {
				t.Fatalf("login: %s", r)
			}
			return map[string]string{"Authorization": l.Token, "Content-Type": "application/json"}
		}
		if c, r := DoH("POST", b+"/entities", `{"name":"quench-smoke-item","description":"from the smoke test"}`, auth()); c/100 != 2 {
			t.Fatalf("create item: %d %s", c, r)
		}
		items := func() string { _, r := DoH("GET", b+"/entities?q=quench-smoke-item", "", auth()); return r }
		t.Contains(items(), "quench-smoke-item", "item listed")
		t.Docker("rm", "-f", name)
		start()
		t.Contains(items(), "quench-smoke-item", "item kept across a restart")
		if regexp.MustCompile(`(?i)read-only file system|panic`).MatchString(t.Logs(name)) {
			t.Fatalf("crashed or wrote to the root\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed (homebox %s: register, login, item kept on the /data volume; user %s)", st.Build.Version, u)
	})
	register("httpd", func(t *T) {
		const b = "http://127.0.0.1:8080/"
		name, body := t.httpUp([]string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-p", "127.0.0.1:8080:8080"}, b, 30)
		t.Match(body, `(?i)httpd|apache`, "default page")
		_, h, _ := getH(httpc, b)
		t.Match(h.Get("Server"), `Apache/2\.4\.[0-9]+`, "Server header")
		if regexp.MustCompile(`(?i)read-only file system|permission denied`).MatchString(t.Logs(name)) {
			t.Fatalf("read-only/permission errors in logs\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed (%s; nonroot user: %s)", h.Get("Server"), t.User("1001"))
	})
	register("longhorn-ui", func(t *T) {
		const b = "http://127.0.0.1:18000/"
		u := t.User("499") // upstream's uid
		mgr := t.hostStub(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"type": "collection", "from": "stub-manager"}`)
		}))
		name := t.Name("")
		t.Start(name, append([]string{"-p", "127.0.0.1:18000:8000", "-e", "LONGHORN_MANAGER_IP=" + mgr}, hostStubFlag...))
		var html string
		t.waitFor(30, time.Second, name, "no index on 8000", func() bool {
			if s := t.Docker("inspect", "-f", "{{.State.Status}}", name); s == "exited" {
				t.Fatalf("nginx exited\n%s", t.Logs(name))
			}
			_, html = Get(b)
			return strings.Contains(strings.ToLower(html), "<html")
		})
		m := regexp.MustCompile(`"\./(main\.[0-9a-f]+(\.async)?\.js)`).FindStringSubmatch(html)
		if m == nil {
			t.Fatalf("index references no script")
		}
		c, bundle := Get(b + m[1])
		if c != 200 || len(bundle) <= 10000 {
			t.Fatalf("bundle %s: HTTP %d, %d bytes", m[1], c, len(bundle))
		}
		if t.Want != "" {
			if _, err := docker("exec", name, "grep", "-rqs", "v"+t.Want, "/web/dist"); err != nil {
				t.Fatalf("bundle not stamped v%s", t.Want)
			}
		}
		_, v1 := Get(b + "v1")
		t.Contains(v1, "stub-manager", "/v1 proxied to the manager")
		t.Logf("smoke test passed (index + bundle on 8000, /v1 proxied to the manager; uid %s)", u)
	})
	register("reloader", func(t *T) {
		u := t.User("1001")
		name := t.Name("")
		t.Docker("create", "--name", name, "-p", "127.0.0.1:19090:9090", "-e", "KUBECONFIG=/kubeconfig", t.Image)
		t.Cleanup(func() { docker("rm", "-f", name) })
		t.Docker("cp", t.DeadKubeconfig(), name+":/kubeconfig")
		t.Docker("start", name)
		var m string
		t.waitFor(60, time.Second, name, "the controllers are not trying the API server", func() bool {
			if s := t.Docker("inspect", "-f", "{{.State.Status}}", name); s != "running" {
				t.Fatalf("reloader exited\n%s", t.Logs(name))
			}
			_, m = Get("http://127.0.0.1:19090/metrics")
			return strings.Contains(m, `rest_client_requests_total{code="<error>",host="127.0.0.1:1"`)
		})
		t.Match(m, `(?m)^reloader_reload_executed_total`, "reloader_ metrics")
		logs := t.Logs(name)
		for _, rt := range []string{"secrets", "configmaps"} {
			t.Contains(logs, "Starting Controller to watch resource type: "+rt, rt+" controller")
		}
		t.Logf("smoke test passed (uid %s, controllers retrying a dead API server, reloader_ metrics)", u)
	})
	register("support-bundle-kit", func(t *T) {
		u := t.rootLikeUpstream()
		v := t.Run("support-bundle-kit", "version")
		if t.Want != "" {
			t.Match(v, `(?m)^v`+regexp.QuoteMeta(t.Want)+` `, "version")
		}
		var mu sync.Mutex
		var bundle []byte
		var path string
		mgr := t.hostStub(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			mu.Lock()
			bundle, path = b, r.URL.Path
			mu.Unlock()
		}))
		name := t.Name("")
		t.Start(name, append([]string{"-e", "SUPPORT_BUNDLE_MANAGER_URL=" + mgr, "-e", "SUPPORT_BUNDLE_COLLECTOR=longhorn", "-e", "SUPPORT_BUNDLE_NODE_NAME=smoke-node"}, hostStubFlag...),
			"support-bundle-collector.sh")
		t.waitFor(60, time.Second, name, "the manager never received a node bundle", func() bool {
			mu.Lock()
			defer mu.Unlock()
			if strings.HasPrefix(path, "/nodes/smoke-node") {
				return true
			}
			if s := t.Docker("inspect", "-f", "{{.State.Status}}", name); s == "exited" {
				t.Fatalf("collector exited before uploading\n%s", t.Logs(name))
			}
			return false
		})
		mu.Lock()
		zr, err := zip.NewReader(bytes.NewReader(bundle), int64(len(bundle)))
		mu.Unlock()
		if err != nil {
			t.Fatalf("upload is not a zip: %v", err)
		}
		found := false
		for _, f := range zr.File {
			found = found || f.Name == "smoke-node/hostinfos/hostinfo"
		}
		if !found {
			t.Fatalf("bundle lacks smoke-node/hostinfos/hostinfo")
		}
		if regexp.MustCompile(`No such file or directory.*common`).MatchString(t.Logs(name)) {
			t.Fatalf("collector could not source its common functions")
		}
		t.Logf("smoke test passed (node collector bundled %d entries and uploaded them; user %q)", len(zr.File), u)
	})
}
