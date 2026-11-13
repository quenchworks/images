package smoke

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func init() {
	register("traefik", func(t *T) {
		t.Run("version")
		const b = "http://127.0.0.1:8080"
		name, _ := t.httpUp([]string{"--read-only", "-p", "127.0.0.1:8080:8080"}, b+"/ping", 30,
			"--ping=true", "--api.dashboard=true", "--api.insecure=true", "--entrypoints.traefik.address=:8080", "--log.level=INFO")
		for _, p := range []string{"/dashboard/", "/api/overview"} {
			if c, _ := Get(b + p); c != 200 {
				t.Fatalf("%s returned %d\n%s", p, c, t.Logs(name))
			}
		}
		t.Logf("smoke test passed (nonroot user: %s, read-only rootfs, ping + dashboard + api)", t.User("1001"))
	})
	register("victorialogs", func(t *T) {
		const b = "http://127.0.0.1:9428"
		name, h := t.httpUp([]string{"--read-only", "--tmpfs", "/victoria-logs-data:rw,mode=1777", "-p", "127.0.0.1:9428:9428"}, b+"/health", 30)
		if h != "OK" {
			t.Fatalf("/health: %s", h)
		}
		if c, r := Post(b+"/insert/jsonline", "application/stream+json", `{"_msg":"qw_smoke_log","source":"smoke"}`+"\n"); c/100 != 2 {
			t.Fatalf("ingest: %d %s", c, r)
		}
		t.waitFor(20, time.Second, name, "query did not return the ingested log", func() bool {
			_, r := Post(b+"/select/logsql/query", "application/x-www-form-urlencoded", url.Values{"query": {"_time:1d qw_smoke_log"}}.Encode())
			return strings.Contains(r, "qw_smoke_log")
		})
		t.Logf("%s", t.Docker("exec", name, "/usr/bin/victoria-logs", "-version"))
		t.Logf("smoke test passed (nonroot user: %s, read-only rootfs, ingest+query)", t.User("1001"))
	})
	register("victoriametrics", func(t *T) {
		const b = "http://127.0.0.1:8428"
		name, h := t.httpUp([]string{"--read-only", "--tmpfs", "/storage:rw,mode=1777", "-p", "127.0.0.1:8428:8428"}, b+"/health", 30)
		if h != "OK" {
			t.Fatalf("/health: %s", h)
		}
		if c, r := Post(b+"/api/v1/import/prometheus", "text/plain", "qw_smoke_metric 123"); c/100 != 2 {
			t.Fatalf("import: %d %s", c, r)
		}
		Get(b + "/internal/force_flush")
		t.waitFor(15, time.Second, name, "query did not return the written sample", func() bool {
			_, r := Post(b+"/api/v1/query", "application/x-www-form-urlencoded", url.Values{"query": {"qw_smoke_metric[5m]"}}.Encode())
			return strings.Contains(r, `"123"`)
		})
		t.Logf("%s", t.Docker("exec", name, "/usr/bin/victoria-metrics", "-version"))
		t.Logf("smoke test passed (nonroot user: %s, read-only rootfs, write+query)", t.User("1001"))
	})
	register("zoraxy", func(t *T) {
		u := t.User("1001")
		v := t.Run("-version")
		if t.Want != "" {
			t.Contains(v, t.Want, "version")
		}
		name := t.Name("")
		t.Start(name, []string{"-p", "127.0.0.1::8000"})
		b := "http://127.0.0.1:" + t.hostPort(name, "8000/tcp")
		jar, _ := cookiejar.New(nil)
		c := &http.Client{Timeout: 10 * time.Second, Jar: jar}
		var login string
		t.waitFor(60, time.Second, name, "panel did not come up", func() bool {
			code, _, body := getH(c, b+"/login.html")
			login = body
			return code == 200
		})
		m := regexp.MustCompile(`name="zoraxy.csrf.Token" content="([^"]*)"`).FindStringSubmatch(login)
		if m == nil || m[1] == "" {
			t.Fatalf("no CSRF token on the login page")
		}
		post := func(path, csrf string) (int, string) {
			req, _ := http.NewRequest("POST", b+path, strings.NewReader("username=admin&password=quench-pass-1"))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Origin", b)
			req.Header.Set("Referer", b+"/login.html")
			if csrf != "" {
				req.Header.Set("X-CSRF-Token", csrf)
			}
			r, err := c.Do(req)
			if err != nil {
				return 0, err.Error()
			}
			defer r.Body.Close()
			body, _ := io.ReadAll(r.Body)
			return r.StatusCode, string(body)
		}
		if code, _ := post("/api/auth/register", ""); code != 403 {
			t.Fatalf("register without a CSRF token got HTTP %d, want 403", code)
		}
		for _, p := range []string{"/api/auth/register", "/api/auth/login"} {
			if _, out := post(p, m[1]); strings.Contains(out, `"error"`) {
				t.Fatalf("%s failed: %s\n%s", p, out, t.Logs(name))
			}
		}
		_, _, list := getH(c, b+"/api/proxy/list?type=host")
		if !strings.HasPrefix(list, "[") {
			t.Fatalf("proxy list is not a JSON array: %s", list)
		}
		t.Logf("smoke test passed (%s, CSRF enforced, register + login, proxy list; nonroot user: %s)", v, u)
	})
	register("checkov", func(t *T) {
		u := t.User("1001")
		if v := t.RunAny("--version"); t.Want != "" && v != t.Want {
			t.Fatalf("checkov %s, expected %s", v, t.Want)
		}
		w := t.TempDir()
		tf := func(acl string) []byte {
			return []byte("variable \"acl\" {\n  default = \"" + acl + "\"\n}\n\nresource \"aws_s3_bucket\" \"b\" {\n  bucket = \"x\"\n  acl    = var.acl\n}\n")
		}
		for d, b := range map[string][]byte{"public": tf("public-read"), "private": tf("private"),
			"k8s": []byte("apiVersion: v1\nkind: Pod\nmetadata:\n  name: p\nspec:\n  containers:\n    - name: c\n      image: busybox\n      securityContext:\n        privileged: true\n")} {
			os.Mkdir(filepath.Join(w, d), 0o755)
			f := "main.tf"
			if d == "k8s" {
				f = "pod.yaml"
			}
			os.WriteFile(filepath.Join(w, d, f), b, 0o644)
		}
		summary := func(dir, fw, check string) [2]int {
			out, _ := docker("run", "--rm", "--read-only", "--tmpfs", "/tmp", "--tmpfs", "/home/nonroot:uid=1001,gid=1001", "-v", w+":/src:ro", t.Image,
				"-d", "/src/"+dir, "--framework", fw, "--check", check, "-o", "json")
			type rep struct{ Summary struct{ Failed, Passed int } }
			var one rep
			var many []rep
			if json.Unmarshal([]byte(out), &many) == nil && len(many) > 0 {
				one = many[0]
			} else if json.Unmarshal([]byte(out), &one) != nil {
				t.Fatalf("checkov %s: no JSON report: %.400s", dir, out)
			}
			return [2]int{one.Summary.Failed, one.Summary.Passed}
		}
		if s := summary("public", "terraform", "CKV_AWS_20"); s != [2]int{1, 0} {
			t.Fatalf("public-read ACL was not flagged: %v", s)
		}
		if s := summary("private", "terraform", "CKV_AWS_20"); s != [2]int{0, 1} {
			t.Fatalf("private ACL did not pass: %v", s)
		}
		if s := summary("k8s", "kubernetes", "CKV_K8S_16"); s != [2]int{1, 0} {
			t.Fatalf("privileged pod was not flagged: %v", s)
		}
		t.Logf("smoke test passed (uid %s, terraform variable rendering + kubernetes)", u)
	})
	register("cockroachdb", func(t *T) {
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/cockroach/cockroach-data:rw,mode=1777,exec", "-p", "18080:8080", "-p", "26257:26257"})
		if uid := t.Docker("exec", name, "id", "-u"); uid != "1001" {
			t.Fatalf("not running as uid 1001 (%s)", uid)
		}
		t.waitCode("http://localhost:18080/health?ready=1", 60, time.Second, name, 200)
		out, err := docker("exec", name, "cockroach", "sql", "--insecure", "--host=localhost:26257", "--format=tsv", "-e", `
CREATE DATABASE IF NOT EXISTS gate;
CREATE TABLE IF NOT EXISTS gate.t (id INT PRIMARY KEY, v STRING);
INSERT INTO gate.t VALUES (1, 'quench') ON CONFLICT (id) DO UPDATE SET v = excluded.v;
SELECT v FROM gate.t WHERE id = 1;`)
		if err != nil || !regexp.MustCompile(`(?m)^quench$`).MatchString(out) {
			t.Fatalf("SQL round trip: %s\n%s", out, t.Logs(name))
		}
		t.Logf("smoke test passed (uid 1001, SQL round trip)")
	})
	register("orientdb", func(t *T) {
		const pass, api = "quenchtest123", "http://root:quenchtest123@127.0.0.1:2480"
		u := t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"-e", "ORIENTDB_ROOT_PASSWORD=" + pass, "-p", "127.0.0.1:2480:2480", "-p", "127.0.0.1:2424:2424"})
		up := func() {
			t.waitFor(90, 2*time.Second, name, "orientdb did not become ready", func() bool { c, _ := Get(api + "/listDatabases"); return c == 200 })
		}
		sql := func(cmd string) string {
			_, b := Post(api+"/command/smoke/sql", "application/json", `{"command":"`+cmd+`"}`)
			return b
		}
		up()
		if strings.Contains(strings.ToLower(t.Logs(name)), "hazelcast") {
			t.Fatalf("hazelcast referenced at boot\n%s", t.Logs(name))
		}
		if c, r := Post(api+"/database/smoke/plocal", "", ""); c/100 != 2 {
			t.Fatalf("database create failed: %d %s", c, r)
		}
		sql("CREATE CLASS Thing")
		sql("INSERT INTO Thing SET name = 'quench'")
		t.Contains(sql("SELECT name FROM Thing"), `"quench"`, "select")
		if c, _ := Get("http://root:wrong@127.0.0.1:2480/database/smoke"); c != 401 {
			t.Fatalf("expected 401 for a wrong password, got %d", c)
		}
		t.Docker("restart", name)
		up()
		t.Contains(sql("SELECT name FROM Thing"), `"quench"`, "record kept across a restart")
		t.Logf("smoke test passed (SQL round trip, auth, restart; user %s)", u)
	})
}
