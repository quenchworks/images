package smoke

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func init() {
	register("perl", func(t *T) {
		if t.Want == "" {
			t.Fatalf("usage: qw smoke perl <image-ref> <major>")
		}
		major := strings.SplitN(t.Want, ".", 2)[0]
		ro := []string{"--read-only", "--tmpfs", "/tmp"}
		v, _ := t.RunFlags(ro, "-v")
		t.Contains(v, "perl "+major+",", "perl -v")
		if out, _ := t.RunFlags(ro, "-e", `print "quench-perl"`); out != "quench-perl" {
			t.Fatalf("-e output wrong: '%s'", out)
		}
		mods, _ := t.RunFlags(ro, "-e", `use strict; use warnings; use Data::Dumper; print "mods-ok\n"`)
		t.Contains(mods, "mods-ok", "core modules")
		if uid, _ := t.RunFlags(ro, "-e", `print $<`); uid != "1001" {
			t.Fatalf("expected uid 1001, got '%s'", uid)
		}
		t.User("1001")
		w := t.TempDir()
		os.WriteFile(filepath.Join(w, "h.pl"), []byte("use strict; use warnings;\nprint \"quench-perl-script\\n\";\n"), 0o644)
		if out, _ := t.RunFlags(append(ro, "-v", w+":/work:ro", "-w", "/work"), "h.pl"); out != "quench-perl-script" {
			t.Fatalf(".pl run output wrong: '%s'", out)
		}
		if _, err := t.RunFlags(ro, "-e", `open(my $f, ">", "/should-fail") or exit 1; exit 0`); err == nil {
			t.Fatalf("rootfs was writable, expected read-only")
		}
		t.Logf("smoke test passed (Perl %s, read-only rootfs)", major)
	})
	register("squid", func(t *T) {
		v := strings.SplitN(t.RunEntryAny("/usr/bin/squid", "-v"), "\n", 2)[0]
		t.Match(v, `Squid Cache: Version [0-9]+\.[0-9]+`, "squid -v")
		if t.Want != "" {
			t.Contains(v, "Version "+t.Want, "version")
		}
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-p", "127.0.0.1:3128:3128"})
		pu, _ := url.Parse("http://127.0.0.1:3128")
		pc := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(pu)}}
		var h http.Header
		t.waitFor(45, time.Second, name, "squid never answered a proxied request", func() bool {
			if s := t.Docker("inspect", "-f", "{{.State.Status}}", name); s != "running" {
				t.Fatalf("container died during startup\n%s", t.Logs(name))
			}
			_, h, _ = getH(pc, "http://quench-smoke.invalid/")
			return h != nil && h.Get("X-Squid-Error") != ""
		})
		t.Match(h.Get("Server"), `(?i)^squid`, "Server: squid")
		t.Match(h.Get("X-Squid-Error"), `^ERR_DNS_FAIL`, "ERR_DNS_FAIL")
		if c, err := net.DialTimeout("tcp", "127.0.0.1:3128", 5*time.Second); err == nil {
			c.SetDeadline(time.Now().Add(10 * time.Second))
			fmt.Fprint(c, "CONNECT quench-smoke.invalid:8443 HTTP/1.1\r\nHost: quench-smoke.invalid:8443\r\n\r\n")
			c.Read(make([]byte, 512))
			c.Close()
		}
		t.waitFor(10, time.Second, name, "CONNECT to :8443 was not denied with 403", func() bool {
			return regexp.MustCompile(`TCP_DENIED/403.*CONNECT quench-smoke\.invalid:8443`).MatchString(t.Logs(name))
		})
		if m := regexp.MustCompile(`read-only file system|[Pp]ermission denied|FATAL|ERROR:`).FindString(t.Logs(name)); m != "" {
			t.Fatalf("errors in logs (%s)\n%s", m, t.Logs(name))
		}
		t.Logf("smoke test passed (nonroot %s, read-only rootfs, squid answered as a proxy)", t.User("1001"))
	})
	register("zabbix-web", func(t *T) {
		if t.Want == "" {
			t.Fatalf("usage: qw smoke zabbix-web <image-ref> <version>")
		}
		u := t.User("1001")
		net_, pg, web := t.Name("net"), t.Name("pg"), t.Name("")
		pw := t.Name("pw")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Cleanup(func() { docker("rm", "-f", pg) })
		t.Docker("run", "-d", "--name", pg, "--network", net_, "-e", "POSTGRES_PASSWORD="+pw, "-e", "POSTGRES_DB=zabbix", "ghcr.io/quenchworks/images/postgresql:18.6")
		t.waitFor(60, time.Second, pg, "postgres did not become ready", func() bool {
			o, _ := docker("exec", "-e", "PGPASSWORD="+pw, pg, "psql", "-h", "/var/run/postgresql", "-U", "postgres", "-d", "zabbix", "-qtAc", "select 1")
			return o == "1"
		})
		db := []string{"--network", net_, "-e", "ZBX_DB_HOST=" + pg, "-e", "ZBX_DB_USER=postgres", "-e", "ZBX_DB_PASSWORD=" + pw}
		initDB := func() string {
			out, _ := t.RunFlags(append(db, "--entrypoint", "/usr/bin/php"), "/usr/share/zabbix-init/init-db.php")
			return out
		}
		t.Contains(initDB(), "loaded data.sql", "loader loaded the schema")
		t.Contains(initDB(), "nothing to load", "loader is idempotent")
		t.Start(web, append(db, "--read-only", "--tmpfs", "/tmp:uid=1001,gid=1001,mode=1777", "-p", "127.0.0.1:18080:8080"))
		api := func(body string) string {
			_, b := DoH("POST", "http://127.0.0.1:18080/api_jsonrpc.php", body, map[string]string{"Content-Type": "application/json-rpc"})
			return b
		}
		var ver string
		t.waitFor(60, time.Second, web, "API never answered", func() bool {
			ver = api(`{"jsonrpc":"2.0","method":"apiinfo.version","params":[],"id":1}`)
			return strings.Contains(ver, `"result"`)
		})
		t.Contains(ver, `"result":"`+t.Want+`"`, "API version")
		t.Match(api(`{"jsonrpc":"2.0","method":"user.login","params":{"username":"Admin","password":"zabbix"},"id":2}`), `"result":"[0-9a-f]{32}`, "Admin login")
		t.Logf("smoke test passed (zabbix-web %s: API login as Admin on PostgreSQL 18, nonroot %s)", t.Want, u)
	})
	register("code-server", func(t *T) {
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/home/coder:rw,mode=0755,uid=1001,gid=1001,exec", "-p", "18080:8080", "-e", "PASSWORD=smokepass123"})
		if uid := t.Docker("exec", name, "id", "-u"); uid != "1001" {
			t.Fatalf("not running as uid 1001 (%s)", uid)
		}
		t.waitFor(60, time.Second, name, "code-server did not serve the login page", func() bool {
			_, b := Get("http://localhost:18080/login")
			return regexp.MustCompile(`(?i)code-server|password`).MatchString(b)
		})
		r, err := noRedirect.Get("http://localhost:18080/")
		if err != nil {
			t.Fatalf("GET /: %v", err)
		}
		r.Body.Close()
		if c := r.StatusCode; c != 302 && c != 303 && c != 200 {
			t.Fatalf("unexpected status %d from /", c)
		}
		t.Logf("smoke test passed (uid 1001, login page, editor requires auth: HTTP %d)", r.StatusCode)
	})
	register("gatus", func(t *T) {
		u := t.User("1001")
		net_, up, name := t.Name("net"), t.Name("up"), t.Name("")
		w := t.TempDir()
		os.WriteFile(filepath.Join(w, "config.yaml"), []byte(`endpoints:
  - name: up-backend
    url: http://`+up+`:8000/
    interval: 5s
    conditions: ["[STATUS] == 200", "[BODY] == quench-up"]
  - name: down-backend
    url: http://`+up+`:9999/
    interval: 5s
    client: { timeout: 2s }
    conditions: ["[STATUS] == 200"]
`), 0o644)
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Cleanup(func() { docker("rm", "-f", up) })
		t.Docker("run", "-d", "--name", up, "--network", net_, "--entrypoint", "/usr/bin/caddy", "ghcr.io/quenchworks/images/caddy:2.11.9", "respond", "--listen", ":8000", "-b", "quench-up")
		t.Start(name, []string{"--network", net_, "--read-only", "-v", w + ":/config:ro", "-p", "127.0.0.1:18080:8080"})
		type ep struct {
			Name    string
			Results []struct{ Success bool }
		}
		last := map[string]bool{}
		t.waitFor(30, 2*time.Second, name, "endpoints not both reported", func() bool {
			var eps []ep
			_, b := Get("http://127.0.0.1:18080/api/v1/endpoints/statuses")
			if json.Unmarshal([]byte(b), &eps) != nil {
				return false
			}
			for _, e := range eps {
				if len(e.Results) > 0 {
					last[e.Name] = e.Results[len(e.Results)-1].Success
				}
			}
			_, a := last["up-backend"]
			_, b2 := last["down-backend"]
			return a && b2
		})
		if !last["up-backend"] || last["down-backend"] {
			t.Fatalf("up-backend not healthy or down-backend not failing: %v", last)
		}
		_, h := Get("http://127.0.0.1:18080/health")
		t.ContainsFold(h, "up", "/health")
		if regexp.MustCompile(`panic|read-only file system`).MatchString(t.Logs(name)) {
			t.Fatalf("crashed or wrote to the root\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed: healthy and failing endpoints reported, /health up, user %s", u)
	})
	register("go", func(t *T) {
		if t.Want == "" {
			t.Fatalf("usage: qw smoke go <image-ref> <minor>")
		}
		minor := strings.Join(strings.SplitN(t.Want, ".", 3)[:2], ".")
		w := t.TempDir()
		os.WriteFile(filepath.Join(w, "go.mod"), []byte("module quench/hello\ngo "+minor+"\n"), 0o644)
		os.WriteFile(filepath.Join(w, "main.go"), []byte("package main\nimport \"fmt\"\nfunc main() { fmt.Print(\"quench-go\") }\n"), 0o644)
		ro := []string{"--read-only", "--tmpfs", "/tmp:exec", "-v", w + ":/src:ro", "-w", "/src"}
		if v, _ := t.RunFlags([]string{"--read-only", "--tmpfs", "/tmp"}, "version"); !strings.HasPrefix(v, "go version go"+minor+".") {
			t.Fatalf("expected 'go version go%s.*', got '%s'", minor, v)
		}
		u := t.User("1001")
		if out, _ := t.RunFlags(ro, "run", "."); out != "quench-go" {
			t.Fatalf("go run output wrong: '%s'", out)
		}
		if out, err := t.RunFlags(ro, "build", "-o", "/tmp/hello", "."); err != nil {
			t.Fatalf("go build: %s", out)
		}
		if out, _ := t.RunFlags(append(ro, "--entrypoint", "/usr/bin/go"), "run", "."); out != "quench-go" {
			t.Fatalf("rebuild output wrong: '%s'", out)
		}
		if _, err := t.RunFlags(ro, "build", "-o", "/should-fail", "."); err == nil {
			t.Fatalf("rootfs was writable, expected read-only")
		}
		t.Logf("smoke test passed (Go %s, user %s, read-only rootfs)", minor, u)
	})
	register("harbor-portal", func(t *T) {
		const b = "http://127.0.0.1:8080"
		name, home := t.httpUp([]string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-p", "127.0.0.1:8080:8080"}, b+"/", 30)
		t.Match(home, `(?i)<!doctype html|<html|<app-root`, "SPA HTML")
		t.WaitHTTP(b+"/harbor", 5, name)
		_, sw := Get(b + "/devcenter-api-2.0")
		t.Match(sw, `(?i)swagger|<html|<!doctype`, "Swagger UI")
		if c, _ := Get(b + "/swagger.json"); c != 200 {
			t.Fatalf("swagger.json not reachable (HTTP %d)", c)
		}
		t.Logf("smoke test passed (nonroot uid: %s, read-only rootfs, SPA + Swagger UI + swagger.json)", t.User("1001"))
	})
}
