package smoke

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/quenchworks/images/tools/qw/internal/probe"
)

func init() {
	register("goaccess", func(t *T) {
		t.User("1001")
		if t.Want != "" {
			t.Contains(t.Run("--version"), "GoAccess - "+t.Want, "version")
		}
		work := t.TempDir()
		log := `10.0.0.1 - - [12/Nov/2026:10:00:00 +0000] "GET / HTTP/1.1" 200 512 "-" "Mozilla/5.0 (X11; Linux x86_64) Firefox/140.0"
10.0.0.1 - - [12/Nov/2026:10:00:01 +0000] "GET /about HTTP/1.1" 200 256 "-" "Mozilla/5.0 (X11; Linux x86_64) Firefox/140.0"
10.0.0.2 - - [12/Nov/2026:10:00:02 +0000] "GET / HTTP/1.1" 200 512 "-" "Mozilla/5.0 (Macintosh) Safari/605.1.15"
10.0.0.2 - - [12/Nov/2026:10:00:03 +0000] "GET /missing HTTP/1.1" 404 128 "-" "Mozilla/5.0 (Macintosh) Safari/605.1.15"
10.0.0.3 - - [12/Nov/2026:10:00:04 +0000] "GET / HTTP/1.1" 200 512 "-" "curl/8.0.0"
10.0.0.3 - - [12/Nov/2026:10:00:05 +0000] "POST /api HTTP/1.1" 201 64 "-" "curl/8.0.0"
`
		os.WriteFile(filepath.Join(work, "access.log"), []byte(log), 0o644)
		for _, out := range []string{"report.json", "report.html"} {
			if o, err := t.RunFlags([]string{"--read-only", "-v", work + ":/w"}, "/w/access.log", "--log-format=COMBINED", "-o", "/w/"+out); err != nil {
				t.Fatalf("goaccess -o %s: %v %s", out, err, o)
			}
		}
		var r struct {
			General struct {
				TotalRequests  int `json:"total_requests"`
				UniqueVisitors int `json:"unique_visitors"`
				FailedRequests int `json:"failed_requests"`
			} `json:"general"`
		}
		b, _ := os.ReadFile(filepath.Join(work, "report.json"))
		if json.Unmarshal(b, &r) != nil {
			t.Fatalf("report.json is not JSON")
		}
		g := r.General
		t.Logf("total_requests %d unique_visitors %d failed %d", g.TotalRequests, g.UniqueVisitors, g.FailedRequests)
		if g.TotalRequests != 6 || g.UniqueVisitors != 3 || g.FailedRequests != 0 {
			t.Fatalf("report counts wrong")
		}
		h, _ := os.ReadFile(filepath.Join(work, "report.html"))
		t.Contains(string(h), "<html", "HTML report")
		t.Logf("smoke test passed")
	})
	register("guacamole", func(t *T) {
		t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp", "-e", "GUACD_HOSTNAME=127.0.0.1", "-p", "127.0.0.1:8080:8080"})
		for i := 0; ; i++ {
			if code, _ := Get("http://127.0.0.1:8080/guacamole/"); code == 200 {
				break
			}
			if i == 60 {
				t.Fatalf("guacamole did not serve /guacamole/\n%s", t.Logs(name))
			}
			time.Sleep(2 * time.Second)
		}
		_, langs := Get("http://127.0.0.1:8080/guacamole/api/languages")
		t.Contains(langs, `"en":"English"`, "languages API")
		logs := t.Logs(name)
		t.Contains(logs, `Extension "PostgreSQL Authentication" (postgresql) loaded`, "postgresql extension")
		t.Contains(logs, "Property postgresql-database is required", "extension read environment properties")
		t.Logf("smoke test passed")
	})
	register("nats-server-config-reloader", func(t *T) {
		t.User("1001")
		ver, _ := t.RunFlags([]string{"--read-only"}, "-version")
		t.WantVersion(ver)
		work := t.TempDir()
		conf, run := filepath.Join(work, "conf"), filepath.Join(work, "run")
		os.MkdirAll(conf, 0o777)
		os.MkdirAll(run, 0o777)
		os.Chmod(conf, 0o777)
		os.Chmod(run, 0o777)
		cfg := filepath.Join(conf, "nats.conf")
		os.WriteFile(cfg, []byte("port: 4222\nhttp_port: 8222\nmax_payload: 1048576\n"), 0o666)
		os.Chmod(cfg, 0o666)
		nats, name := t.Name("nats"), t.Name("")
		t.Cleanup(func() { docker("rm", "-f", nats) })
		t.Docker("run", "-d", "--name", nats, "-v", conf+":/etc/nats-config", "-v", run+":/var/run/nats",
			"ghcr.io/quenchworks/images/nats:2.15.0", "--config", "/etc/nats-config/nats.conf", "-P", "/var/run/nats/nats.pid")
		pid := filepath.Join(run, "nats.pid")
		for i := 0; i < 30; i++ {
			if fi, err := os.Stat(pid); err == nil && fi.Size() > 0 {
				break
			}
			time.Sleep(time.Second)
		}
		if fi, err := os.Stat(pid); err != nil || fi.Size() == 0 {
			t.Fatalf("nats wrote no pid file\n%s", t.Logs(nats))
		}
		// the reloader shares nats's PID namespace and signals it when the config changes
		t.Start(name, []string{"--read-only", "--pid", "container:" + nats, "-v", conf + ":/etc/nats-config:ro", "-v", run + ":/var/run/nats:ro"},
			"-pid", "/var/run/nats/nats.pid", "-config", "/etc/nats-config/nats.conf")
		time.Sleep(3 * time.Second)
		os.WriteFile(cfg, []byte("port: 4222\nhttp_port: 8222\nmax_payload: 2097152\n"), 0o666)
		for i := 0; ; i++ {
			if strings.Contains(strings.ToLower(t.Logs(nats)), "reloaded") {
				break
			}
			if i == 30 {
				t.Fatalf("nats never reloaded\n--- reloader\n%s\n--- nats\n%s", t.Logs(name), t.Logs(nats))
			}
			time.Sleep(time.Second)
		}
		t.Logf("smoke test passed")
	})
	register("podman", func(t *T) {
		t.User("1001")
		ver := t.Run("--version")
		t.Logf("%s", ver)
		if t.Want != "" && ver != "podman version "+t.Want {
			t.Fatalf("expected %s", t.Want)
		}
		p := []string{"--storage-driver", "vfs", "--cgroup-manager", "cgroupfs", "--events-backend", "file"}
		out, err := t.RunFlags([]string{"--privileged"}, append(p, "run", "--rm", "--network", "none", "ghcr.io/quenchworks/images/crane:0.22.1", "version")...)
		if err != nil {
			t.Fatalf("podman run failed:\n%s", out)
		}
		t.Contains(out, "0.22.1", "inner container")
		sizes, _ := t.RunFlags([]string{"--privileged"}, append(p, "info", "--format", "{{range .Host.IDMappings.UIDMap}}{{.Size}} {{end}}")...)
		t.Logf("uid map sizes: %s", sizes)
		t.Contains(sizes, "65536", "subuid range mapped")
		t.Logf("smoke test passed")
	})
	register("rethinkdb", func(t *T) {
		t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"-p", "127.0.0.1:28015:28015", "-p", "127.0.0.1:18080:8080"})
		t.WaitLog(name, "Server ready", 60)
		if code, _ := Get("http://127.0.0.1:18080/"); code != 200 {
			t.Fatalf("web admin not served (%d)", code)
		}
		if probe.RethinkSmoke([]string{"--addr", "127.0.0.1:28015", "--want", t.Want}) != 0 {
			t.Fatalf("driver round trip failed")
		}
		t.Logf("smoke test passed")
	})
	register("tigerbeetle", func(t *T) {
		t.Logf("version: %s", t.Run("version"))
		if uid := t.RunEntry("/bin/sh", "-c", "id -u"); uid != "1001" {
			t.Fatalf("not running as uid 1001: %s", uid)
		}
		// format a data file on a read-only root with a writable tmpfs /data
		if out, err := t.RunFlags([]string{"--security-opt", "seccomp=unconfined", "--read-only", "--tmpfs", "/data:rw,mode=0700,uid=1001,gid=1001"},
			"format", "--cluster=0", "--replica=0", "--replica-count=1", "/data/0_0.tigerbeetle"); err != nil {
			t.Fatalf("format: %v %s", err, out)
		}
		t.Logf("smoke test passed")
	})
	register("backing-image-manager", func(t *T) {
		// root, like upstream
		if u := t.Docker("inspect", t.Image, "--format", "{{.Config.User}}"); u != "0" && u != "" {
			t.Fatalf("expected root like upstream, got '%s'", u)
		}
		t.WantVersionV(t.RunEntry("backing-image-manager", "version", "--client-only"))
		name := t.Name("")
		t.Start(name, []string{"--tmpfs", "/data", "--entrypoint", "sh"}, "-c",
			`echo "{\"diskUUID\":\"smoke-disk\"}" > /data/longhorn-disk.cfg && exec backing-image-manager --debug daemon --listen :8000 --sync-listen :8001 --disk-uuid smoke-disk`)
		t.WaitLog(name, "Backing Image Manager listening to", 30)
		t.WaitLog(name, "sync server of backing Image Manager listening", 30)
		time.Sleep(3 * time.Second)
		if st := t.Docker("inspect", "-f", "{{.State.Status}}", name); st != "running" {
			t.Fatalf("daemon exited\n%s", t.Logs(name))
		}
		t.Docker("exec", name, "sh", "-c", `for t in qemu-img losetup dd sync; do command -v $t >/dev/null || { echo "missing $t"; exit 1; }; done`)
		t.Logf("smoke test passed")
	})
	register("cloudflared", func(t *T) {
		t.User("1001")
		if t.Want != "" {
			t.Contains(t.Run(), "cloudflared version "+t.Want+" ", "version")
		}
		// a well-formed but fake tunnel token: it cannot connect, but the metrics server must serve
		tok := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf(`{"a":"00000000000000000000000000000000","t":"11111111-1111-1111-1111-111111111111","s":"%s"}`,
			base64.StdEncoding.EncodeToString([]byte("smoke-secret-0123456789abcdef0123")))))
		name := t.Name("")
		t.Start(name, []string{"--read-only", "-p", "127.0.0.1:12000:2000"}, "tunnel", "--no-autoupdate", "--metrics", "0.0.0.0:2000", "run", "--token", tok)
		var m string
		for i := 0; ; i++ {
			if _, m = Get("http://127.0.0.1:12000/metrics"); strings.Contains(m, "\nbuild_info{") || strings.HasPrefix(m, "build_info{") {
				break
			}
			if i == 30 {
				t.Fatalf("metrics server never answered\n%s", t.Logs(name))
			}
			time.Sleep(time.Second)
		}
		if t.Want != "" {
			t.Match(m, `(?m)^build_info\{.*version="`+t.Want+`"`, "build_info version")
		}
		t.Logf("smoke test passed")
	})
	register("configurable-http-proxy", func(t *T) {
		t.User("1001")
		ver := t.RunEntry("/usr/bin/node", "/opt/chp/node_modules/configurable-http-proxy/bin/configurable-http-proxy", "--version")
		t.Logf("configurable-http-proxy %s", ver)
		if t.Want != "" && ver != t.Want {
			t.Fatalf("expected %s", t.Want)
		}
		name := t.Name("")
		t.Start(name, []string{"-e", "CONFIGPROXY_AUTH_TOKEN=smoke", "-p", "127.0.0.1:8000:8000", "-p", "127.0.0.1:8001:8001"})
		a := map[string]string{"Authorization": "token smoke"}
		const api = "http://127.0.0.1:8001/api/routes"
		for i := 0; ; i++ {
			if code, _ := DoH("GET", api, "", a); code == 200 {
				break
			}
			if i == 30 {
				t.Fatalf("proxy API never answered\n%s", t.Logs(name))
			}
			time.Sleep(time.Second)
		}
		if code, body := DoH("POST", api+"/smoke", `{"target":"http://127.0.0.1:9"}`, a); code >= 300 {
			t.Fatalf("add route: %d %s", code, body)
		}
		_, routes := DoH("GET", api, "", a)
		t.Contains(routes, `"/smoke"`, "stored route")
		if code, _ := DoH("GET", api, "", map[string]string{"Authorization": "token wrong"}); code != 403 {
			t.Fatalf("API accepted a wrong token (%d)", code)
		}
		t.Logf("smoke test passed")
	})
	register("coroot", func(t *T) {
		t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"-p", "18080:8080"})
		t.WaitHTTP("http://127.0.0.1:18080/", 60, name)
		t.Match(t.Logs(name), `(?i)version:\s*[0-9]+\.[0-9]+\.[0-9]+`, "version not stamped")
		t.Logf("smoke test passed")
	})
}
