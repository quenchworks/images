package smoke

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var pgaCSRF = regexp.MustCompile(`(?:["']csrfToken["']\s*:|pgAdmin\['csrf_token'\]\s*=)\s*["']([^"']+)["']`)

func init() {
	// Against the catalog's PostgreSQL, on a read-only root with a data volume: the entrypoint creates the config DB
	// and the first user, gunicorn answers /misc/ping, a wrong password does not reach the browser, the right one
	// does, and a server registered with connect_now connects to PostgreSQL. The config DB survives a restart.
	register("pgadmin4", func(t *T) {
		const base = "http://127.0.0.1:18122"
		const dbpw, email, pw = "Smoke-Db-2026", "admin@example.com", "Smoke-pgAdmin-2026!"
		u := t.User("1001")
		net := t.Name("net")
		t.Docker("network", "create", net)
		t.Cleanup(func() { docker("network", "rm", net) })
		db := t.Name("db")
		t.Docker("run", "-d", "--name", db, "--network", net, "--network-alias", "db", "-e", "POSTGRES_PASSWORD="+dbpw, leantimePostgres)
		t.Cleanup(func() { docker("rm", "-f", db) })
		t.waitFor(60, 2*time.Second, db, "postgres did not become ready", func() bool {
			_, err := docker("exec", "-e", "PGPASSWORD="+dbpw, db, "psql", "-h", "127.0.0.1", "-U", "postgres", "-tAc", "select 1")
			return err == nil
		})
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", "-f", vol) })
		flags := []string{"--network", net, "--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/var/lib/pgadmin",
			"-p", "127.0.0.1:18122:8080", "-e", "PGADMIN_DEFAULT_EMAIL=" + email, "-e", "PGADMIN_DEFAULT_PASSWORD=" + pw,
			"-e", "PGADMIN_CONFIG_MASTER_PASSWORD_REQUIRED=False"}
		name := t.Name("")
		t.Start(name, flags)
		up := func() {
			t.waitFor(90, 2*time.Second, name, "/misc/ping did not answer", func() bool {
				c, b := Get(base + "/misc/ping")
				return c == http.StatusOK && strings.Contains(b, "PING")
			})
		}
		up()
		login := func(pass string) (*http.Client, bool) {
			jar, _ := cookiejar.New(nil)
			cl := &http.Client{Jar: jar, Timeout: 30 * time.Second}
			r, err := cl.Get(base + "/login")
			if err != nil {
				t.Fatalf("login page: %v", err)
			}
			b, _ := io.ReadAll(r.Body)
			r.Body.Close()
			m := pgaCSRF.FindSubmatch(b)
			if m == nil {
				t.Fatalf("no CSRF token on the login page (%d)", r.StatusCode)
			}
			r, err = cl.PostForm(base+"/authenticate/login", url.Values{"email": {email}, "password": {pass}, "csrf_token": {string(m[1])}})
			if err != nil {
				t.Fatalf("login: %v", err)
			}
			r.Body.Close()
			cl.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
			r, err = cl.Get(base + "/browser/")
			if err != nil {
				t.Fatalf("browser: %v", err)
			}
			r.Body.Close()
			cl.CheckRedirect = nil
			return cl, r.StatusCode == http.StatusOK
		}
		if _, ok := login("wrong-password-1"); ok {
			t.Fatalf("a wrong password reached the browser")
		}
		cl, ok := login(pw)
		if !ok {
			t.Fatalf("the right password did not reach the browser\n%s", t.Logs(name))
		}
		r, err := cl.Get(base + "/browser/js/utils.js")
		if err != nil {
			t.Fatalf("utils.js: %v", err)
		}
		js, _ := io.ReadAll(r.Body)
		r.Body.Close()
		m := pgaCSRF.FindSubmatch(js)
		if m == nil {
			t.Fatalf("no CSRF token in utils.js (%d)", r.StatusCode)
		}
		req, _ := http.NewRequest("POST", base+"/browser/server/obj/1/", strings.NewReader(
			`{"name":"smoke-db","host":"db","port":5432,"db":"postgres","username":"postgres","password":"`+dbpw+`","save_password":true,"connect_now":true,"sslmode":"prefer"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-pgA-CSRFToken", string(m[1]))
		r, err = cl.Do(req)
		if err != nil {
			t.Fatalf("add server: %v", err)
		}
		body, _ := io.ReadAll(r.Body)
		r.Body.Close()
		if r.StatusCode != http.StatusOK || !strings.Contains(string(body), `"connected": true`) && !strings.Contains(string(body), `"connected":true`) {
			t.Fatalf("add server with connect_now: %d %s\n%s", r.StatusCode, body, t.Logs(name))
		}
		// the config DB (users, servers) lives on the volume
		t.Docker("restart", name)
		up()
		if _, ok := login(pw); !ok {
			t.Fatalf("the user is gone after a restart\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed (pgadmin4 %s: read-only gunicorn, setup-db, wrong password refused, server added and connected to PostgreSQL, user kept across a restart; user %s)", t.Want, u)
	})
}
