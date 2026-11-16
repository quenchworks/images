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

// kimaiMariaDB is the database the smoke test runs Kimai against (the catalog's own image, from the lock).
const kimaiMariaDB = "ghcr.io/quenchworks/images/mariadb@sha256:ccb271d7cb5442de64bcb560b23d8fc985287e0052dc0e01094ab7b874808583"

var kimaiCSRF = regexp.MustCompile(`name="_csrf_token" value="([^"]+)"`)

// kimaiLogin signs in through the form and returns the Location of the login_check POST.
func kimaiLogin(base, user, pass string) string {
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	r, err := c.Get(base + "/en/login")
	if err != nil {
		return err.Error()
	}
	b, _ := io.ReadAll(r.Body)
	r.Body.Close()
	m := kimaiCSRF.FindStringSubmatch(string(b))
	if m == nil {
		return "no _csrf_token on /en/login"
	}
	r, err = c.PostForm(base+"/en/login_check", url.Values{"_username": {user}, "_password": {pass}, "_csrf_token": {m[1]}})
	if err != nil {
		return err.Error()
	}
	defer r.Body.Close()
	return r.Header.Get("Location")
}

func init() {
	// On a read-only rootfs against the catalog's MariaDB: kimai:install creates the schema, the admin from
	// ADMIN_USER/ADMIN_PASSWORD signs in, a wrong password is refused, and the account survives a restart.
	register("kimai", func(t *T) {
		const base = "http://127.0.0.1:18085"
		const dbpw = "Smoke-Db-2026"
		const pw = "Smoke-Admin-2026"
		u := t.User("1001")
		net := t.Name("net")
		t.Docker("network", "create", net)
		t.Cleanup(func() { docker("network", "rm", net) })
		db := t.Name("db")
		t.Docker("run", "-d", "--name", db, "--network", net, "--network-alias", "db", "-e", "MARIADB_ROOT_PASSWORD="+dbpw,
			"-e", "MARIADB_DATABASE=kimai", "-e", "MARIADB_USER=kimai", "-e", "MARIADB_PASSWORD="+dbpw, kimaiMariaDB)
		t.Cleanup(func() { docker("rm", "-f", db) })
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		flags := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--network", net, "-v", vol + ":/var/lib/kimai", "-p", "127.0.0.1:18085:8080",
			"-e", "APP_SECRET=quench-smoke-secret-0123456789abcdef",
			"-e", "DATABASE_URL=mysql://kimai:" + dbpw + "@db:3306/kimai?charset=utf8mb4&serverVersion=11.8.10-MariaDB",
			"-e", "ADMIN_USER=admin", "-e", "ADMIN_EMAIL=admin@quench-works.com", "-e", "ADMIN_PASSWORD=" + pw}
		start := func(name string) {
			t.Start(name, flags)
			for i := 0; i < 150; i++ {
				if c, _ := restDo("GET", base+"/en/login", "", "", ""); c == http.StatusOK {
					return
				}
				time.Sleep(2 * time.Second)
			}
			t.Fatalf("kimai did not serve /en/login:\n%s", t.Logs(name))
		}
		name := t.Name("")
		start(name)
		_, page := restDo("GET", base+"/en/login", "", "", "")
		t.Contains(page, "Kimai", "the login page")
		if got := kimaiLogin(base, "admin", "wrong-password-1"); !strings.HasSuffix(got, "/en/login") {
			t.Fatalf("a wrong password was not sent back to /en/login: %q", got)
		}
		if got := kimaiLogin(base, "admin", pw); got == "" || strings.HasSuffix(got, "/login") {
			t.Fatalf("admin sign-in failed: %q\n%s", got, t.Logs(name))
		}
		docker("rm", "-f", name)
		start(t.Name("again"))
		if got := kimaiLogin(base, "admin", pw); got == "" || strings.HasSuffix(got, "/login") {
			t.Fatalf("admin sign-in after a restart failed: %q", got)
		}
		t.Logf("smoke test passed (kimai %s: read-only boot against MariaDB, schema installed, admin from the environment signs in, wrong password refused, account kept across a restart; user %s)", t.Want, u)
	})
}
