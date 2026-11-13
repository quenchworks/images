package smoke

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// memcached sends one text-protocol request and reads lines until one has prefix end.
func memcached(addr, req, end string) string {
	c, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return ""
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(3 * time.Second))
	fmt.Fprint(c, req)
	var out strings.Builder
	sc := bufio.NewScanner(c)
	for sc.Scan() {
		l := strings.TrimRight(sc.Text(), "\r")
		out.WriteString(l + "\n")
		if strings.HasPrefix(l, end) {
			break
		}
	}
	return out.String()
}

func init() {
	register("memcached", func(t *T) {
		const a = "127.0.0.1:11211"
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp", "-p", a + ":11211"})
		var v string
		t.waitFor(30, time.Second, name, "memcached did not become ready", func() bool { v = memcached(a, "version\r\n", "VERSION"); return strings.HasPrefix(v, "VERSION") })
		t.Contains(memcached(a, "set qw 0 0 5\r\nhello\r\n", "STORED"), "STORED", "SET")
		t.Contains(memcached(a, "get qw\r\n", "END"), "hello", "GET")
		t.Logf("smoke test passed (%s; nonroot user: %s)", strings.TrimSpace(v), t.User("1001"))
	})
	register("neo4j", func(t *T) {
		const pass = "quenchtest123"
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/data:rw,exec,mode=1777", "--tmpfs", "/logs:rw,mode=1777", "--tmpfs", "/conf:rw,mode=1777",
			"--tmpfs", "/var/run/neo4j:rw,mode=1777", "-e", "NEO4J_AUTH=neo4j/" + pass, "-p", "127.0.0.1:7474:7474", "-p", "127.0.0.1:7687:7687"})
		t.waitFor(90, 2*time.Second, name, "neo4j did not become ready", func() bool {
			c, b := Get("http://127.0.0.1:7474/")
			return c == 200 && strings.Contains(b, `"bolt`)
		})
		cy := func(q string) (string, error) {
			return docker("exec", name, "cypher-shell", "-a", "bolt://localhost:7687", "-u", "neo4j", "-p", pass, q)
		}
		if out, err := cy("CREATE (:Thing {name:'quench'}) RETURN 1;"); err != nil {
			t.Fatalf("cypher write failed: %s", out)
		}
		out, _ := cy("MATCH (t:Thing) RETURN t.name;")
		t.Contains(out, "quench", "cypher read")
		t.Logf("smoke test passed (Cypher round trip; nonroot user: %s)", t.User("1001"))
	})
	register("pgbouncer", func(t *T) {
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/var/run/pgbouncer:uid=1001,gid=1001", "-p", "127.0.0.1:6432:6432"})
		t.WaitTCP(30, name, "127.0.0.1:6432")
		t.Match(t.Logs(name), `(?i)process up|listening on`, "startup line")
		if s := t.Docker("inspect", "-f", "{{.State.Status}}", name); s != "running" {
			t.Fatalf("container is not running\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed (nonroot user: %s, read-only rootfs)", t.User("1001"))
	})
	register("poetry", func(t *T) {
		if t.Want == "" {
			t.Fatalf("usage: qw smoke poetry <image-ref> <pyminor>")
		}
		ro := []string{"--read-only", "--tmpfs", "/tmp"}
		py := append(ro, "--entrypoint", "/usr/bin/python3")
		v, _ := t.RunFlags(ro, "--version")
		t.Contains(v, "Poetry (version ", "poetry --version")
		pv, _ := t.RunFlags(py, "--version")
		if !strings.HasPrefix(pv, "Python "+t.Want+".") {
			t.Fatalf("expected 'Python %s.*', got '%s'", t.Want, pv)
		}
		if out, err := t.RunFlags(ro, "config", "--list"); err != nil {
			t.Fatalf("poetry could not use its config under /tmp: %s", out)
		}
		if uid, _ := t.RunFlags(py, "-c", "import os; print(os.getuid())"); uid != "1001" {
			t.Fatalf("expected uid 1001, got '%s'", uid)
		}
		t.User("1001")
		t.Contains(t.Docker("inspect", t.Image, "--format", "{{json .Config.Entrypoint}}"), "/usr/bin/poetry", "entrypoint")
		if _, err := t.RunFlags(py, "-c", `open("/should-fail","w")`); err == nil {
			t.Fatalf("rootfs was writable, expected read-only")
		}
		t.Logf("smoke test passed (%s, %s, read-only rootfs)", v, pv)
	})
	register("shlink", func(t *T) {
		const b = "http://127.0.0.1:18085"
		key := t.Name("key") + "-0123456789abcdef"
		u := t.User("1001")
		t.RunEntry("/etc/shlink/bin/rr", "-v")
		vol, name := t.Name("data"), t.Name("")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		start := func() {
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:uid=1001,gid=1001,mode=1777", "-v", vol + ":/etc/shlink/data",
				"-e", "DEFAULT_DOMAIN=s.example", "-e", "IS_HTTPS_ENABLED=false", "-e", "INITIAL_API_KEY=" + key, "-p", "127.0.0.1:18085:8080"})
			t.WaitHTTP(b+"/rest/health", 90, name)
		}
		start()
		_, hb := Get(b + "/rest/health")
		var h struct{ Version string }
		json.Unmarshal([]byte(hb), &h)
		if h.Version == "" || (t.Want != "" && h.Version != t.Want) {
			t.Fatalf("shlink %q, expected %s", h.Version, t.Want)
		}
		if c, _ := DoH("GET", b+"/rest/v3/short-urls", "", map[string]string{"X-Api-Key": "wrong"}); c != 401 {
			t.Fatalf("a wrong API key got HTTP %d", c)
		}
		if c, r := DoH("POST", b+"/rest/v3/short-urls", `{"longUrl":"https://quench-works.com/smoke","customSlug":"qsmoke"}`,
			map[string]string{"X-Api-Key": key, "Content-Type": "application/json"}); c/100 != 2 {
			t.Fatalf("create short URL: %d %s", c, r)
		}
		redirect := func() string {
			r, _ := hostReq("GET", b+"/qsmoke", "s.example")
			resp, err := noRedirect.Do(r)
			if err != nil {
				return err.Error()
			}
			resp.Body.Close()
			return fmt.Sprintf("%d %s", resp.StatusCode, resp.Header.Get("Location"))
		}
		if r := redirect(); r != "302 https://quench-works.com/smoke" {
			t.Fatalf("the short URL does not redirect: %s", r)
		}
		t.Docker("rm", "-f", name)
		start()
		if r := redirect(); r != "302 https://quench-works.com/smoke" {
			t.Fatalf("short URL lost across a restart: %s", r)
		}
		if regexp.MustCompile(`(?i)read-only file system|PHP Fatal`).MatchString(t.Logs(name)) {
			t.Fatalf("error in the logs\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed (shlink %s: API key, redirect kept on the data volume; user %s)", h.Version, u)
	})
	register("terralist", func(t *T) {
		const b = "http://127.0.0.1:15758"
		u := t.User("1001")
		if v, _ := t.RunFlags([]string{"--read-only", "--tmpfs", "/tmp"}, "version"); t.Want != "" {
			t.Contains(v, t.Want, "version")
		}
		key := t.Name("mk")
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp", "--tmpfs", "/data:uid=1001,gid=1001", "-p", "127.0.0.1:15758:5758"}, "server",
			"--url", b, "--sqlite-path", "/data/terralist.db", "--local-store", "/data/store", "--local-registry", "/data/registry",
			"--modules-storage-resolver", "local", "--providers-storage-resolver", "local",
			"--token-signing-secret", randFrom("abcdef0123456789", 24), "--local-token-signing-secret", randFrom("abcdef0123456789", 24),
			"--cookie-secret", strings.Repeat("c", 32), "--oauth-state-secret", randFrom("abcdef0123456789", 24),
			"--oauth-provider", "github", "--gh-client-id", "smoke", "--gh-client-secret", "smoke", "--master-api-key", key)
		t.waitFor(60, time.Second, name, "terralist never answered service discovery", func() bool {
			_, d := Get(b + "/.well-known/terraform.json")
			return strings.Contains(d, "modules.v1")
		})
		h := map[string]string{"X-API-Key": key, "Content-Type": "application/json"}
		if c, r := DoH("POST", b+"/v1/api/authorities/", `{"name":"quench","public":true,"owner":"smoke@example.com"}`, h); c/100 != 2 {
			t.Fatalf("authority create: %d %s", c, r)
		}
		_, up := DoH("POST", b+"/v1/api/modules/quench/label/null/0.25.0/upload",
			`{"download_url":"https://github.com/cloudposse/terraform-null-label/archive/refs/tags/0.25.0.tar.gz"}`, h)
		t.Contains(up, `"errors":[]`, "module upload")
		_, vs := Get(b + "/v1/modules/quench/label/null/versions")
		t.Contains(vs, `"version":"0.25.0"`, "module version listed")
		_, hd, _ := getH(noRedirect, b+"/v1/modules/quench/label/null/0.25.0/download")
		get := hd.Get("X-Terraform-Get")
		if get == "" {
			t.Fatalf("no X-Terraform-Get")
		}
		_, z := Get(get)
		zr, err := zip.NewReader(bytes.NewReader([]byte(z)), int64(len(z)))
		if err != nil {
			t.Fatalf("module download is not a zip: %v", err)
		}
		found := false
		for _, f := range zr.File {
			found = found || strings.HasSuffix(f.Name, "main.tf")
		}
		if !found {
			t.Fatalf("module zip has no main.tf")
		}
		if m := regexp.MustCompile(`"level":"(error|fatal|panic)"`).FindString(t.Logs(name)); m != "" {
			t.Fatalf("terralist logged errors\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed (uid %s, module uploaded, listed and served)", u)
	})
	register("thanos", func(t *T) {
		t.Run("--version")
		t.Run("query", "--help")
		u := t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,uid=1001,gid=1001", "-p", "127.0.0.1:10902:10902"},
			"query", "--http-address=0.0.0.0:10902", "--grpc-address=0.0.0.0:10901", "--endpoint=127.0.0.1:19090")
		if uid, _ := docker("exec", name, "id", "-u"); uid != "1001" {
			t.Fatalf("expected runtime uid 1001, got '%s'", uid)
		}
		t.WaitHTTP("http://127.0.0.1:10902/-/healthy", 60, name)
		if c, _ := Get("http://127.0.0.1:10902/-/ready"); c != 200 && c != 503 {
			t.Fatalf("/-/ready returned %d", c)
		}
		t.Logf("smoke test passed (nonroot uid: %s, read-only rootfs, query healthy)", u)
	})
}

// hostReq builds a GET with an explicit Host header.
func hostReq(method, url, host string) (*http.Request, error) {
	r, err := http.NewRequest(method, url, nil)
	if err == nil {
		r.Host = host
	}
	return r, err
}
