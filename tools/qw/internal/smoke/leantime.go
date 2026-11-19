package smoke

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

const leantimePostgres = "ghcr.io/quenchworks/images/postgresql@sha256:f2aadd3d9651056f67735ec100ce255fabc86121bfa45f20fa2a06df82886d46"

// leantimeLogin posts the sign-in form as a browser does (the CSRF token from the page, and an Origin header, which
// Leantime's VerifyRequestOrigin checks) and returns where it redirects: back to /auth/login on a failure.
func leantimeLogin(base, email, pass string) string {
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	r, err := c.Get(base + "/auth/login")
	if err != nil {
		return err.Error()
	}
	b, _ := io.ReadAll(r.Body)
	r.Body.Close()
	m := snipeCSRF.FindStringSubmatch(string(b))
	if m == nil {
		return "no _token on /auth/login"
	}
	form := url.Values{"username": {email}, "password": {pass}, "_token": {m[1]}, "redirectUrl": {""}}
	req, _ := http.NewRequest("POST", base+"/auth/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", base)
	r, err = c.Do(req)
	if err != nil {
		return err.Error()
	}
	defer r.Body.Close()
	return r.Header.Get("Location")
}

func init() {
	// On a read-only rootfs against the catalog's PostgreSQL: the init installs the schema and creates the admin from
	// the environment (bin/leantime db:migrate); the admin signs in, a wrong password is refused, and after a restart
	// the same run updates instead of reinstalling and the account still signs in.
	register("leantime", func(t *T) {
		const base = "http://127.0.0.1:18087"
		const dbpw = "Smoke-Db-2026"
		const pw = "Smoke-Admin-2026!"
		const email = "admin@quench-works.com"
		u := t.User("1001")
		net := t.Name("net")
		t.Docker("network", "create", net)
		t.Cleanup(func() { docker("network", "rm", net) })
		db := t.Name("db")
		t.Docker("run", "-d", "--name", db, "--network", net, "--network-alias", "db", "-e", "POSTGRES_PASSWORD="+dbpw,
			"-e", "POSTGRES_USER=leantime", "-e", "POSTGRES_DB=leantime_db", leantimePostgres)
		t.Cleanup(func() { docker("rm", "-f", db) })
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		flags := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--network", net, "-v", vol + ":/var/lib/leantime", "-p", "127.0.0.1:18087:8080",
			"-e", "LEAN_APP_URL=" + base, "-e", "LEAN_SESSION_PASSWORD=quench-leantime-smoke-session-0123456789",
			"-e", "LEAN_DB_DEFAULT_CONNECTION=pgsql", "-e", "LEAN_DB_HOST=db", "-e", "LEAN_DB_PORT=5432",
			"-e", "LEAN_DB_DATABASE=leantime_db", "-e", "LEAN_DB_USER=leantime", "-e", "LEAN_DB_PASSWORD=" + dbpw,
			"-e", "LEAN_ADMIN_EMAIL=" + email, "-e", "LEAN_ADMIN_PASSWORD=" + pw}
		start := func(name string) {
			t.Start(name, flags)
			for i := 0; i < 150; i++ {
				if c, _ := restDo("GET", base+"/auth/login", "", "", ""); c == http.StatusOK {
					return
				}
				time.Sleep(2 * time.Second)
			}
			t.Fatalf("leantime did not serve /auth/login:\n%s", t.Logs(name))
		}
		name := t.Name("")
		start(name)
		_, page := restDo("GET", base+"/auth/login", "", "", "")
		t.Contains(page, "Leantime", "the login page")
		if got := leantimeLogin(base, email, "wrong-password-1"); !strings.Contains(got, "/auth/login") {
			t.Fatalf("a wrong password was not sent back to /auth/login: %q", got)
		}
		if got := leantimeLogin(base, email, pw); got == "" || strings.Contains(got, "/auth/login") {
			t.Fatalf("admin sign-in failed: %q\n%s", got, t.Logs(name))
		}
		docker("rm", "-f", name)
		start(t.Name("again"))
		if got := leantimeLogin(base, email, pw); got == "" || strings.Contains(got, "/auth/login") {
			t.Fatalf("admin sign-in after a restart failed: %q", got)
		}
		t.Logf("smoke test passed (leantime %s: read-only boot against PostgreSQL, schema installed and admin created by db:migrate, admin signs in, wrong password refused, account kept across a restart; user %s)", t.Want, u)
	})
}
