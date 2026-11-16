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

// snipeMariaDB is the database the smoke test runs Snipe-IT against (the catalog's own image, from the lock).
const snipeMariaDB = "ghcr.io/quenchworks/images/mariadb@sha256:ccb271d7cb5442de64bcb560b23d8fc985287e0052dc0e01094ab7b874808583"

var snipeCSRF = regexp.MustCompile(`name="_token" value="([^"]+)"`)

// snipeLogin signs in through the form and returns the Location of the POST.
func snipeLogin(base, user, pass string) string {
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	r, err := c.Get(base + "/login")
	if err != nil {
		return err.Error()
	}
	b, _ := io.ReadAll(r.Body)
	r.Body.Close()
	m := snipeCSRF.FindStringSubmatch(string(b))
	if m == nil {
		return "no _token on /login"
	}
	r, err = c.PostForm(base+"/login", url.Values{"username": {user}, "password": {pass}, "_token": {m[1]}})
	if err != nil {
		return err.Error()
	}
	defer r.Body.Close()
	return r.Header.Get("Location")
}

func init() {
	// On a read-only rootfs against the catalog's MariaDB: the init migrates and creates the admin and settings from
	// the environment; the admin signs in, a wrong password is refused, and the account survives a restart.
	register("snipe-it", func(t *T) {
		const base = "http://127.0.0.1:18086"
		const dbpw = "Smoke-Db-2026"
		const pw = "Smoke-Admin-2026"
		u := t.User("1001")
		net := t.Name("net")
		t.Docker("network", "create", net)
		t.Cleanup(func() { docker("network", "rm", net) })
		db := t.Name("db")
		t.Docker("run", "-d", "--name", db, "--network", net, "--network-alias", "db", "-e", "MARIADB_ROOT_PASSWORD="+dbpw,
			"-e", "MARIADB_DATABASE=snipeit", "-e", "MARIADB_USER=snipeit", "-e", "MARIADB_PASSWORD="+dbpw, snipeMariaDB)
		t.Cleanup(func() { docker("rm", "-f", db) })
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		flags := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--network", net, "-v", vol + ":/var/lib/snipeit", "-p", "127.0.0.1:18086:8080",
			"-e", "APP_KEY=base64:UXVlbmNoU25pcGVTbW9rZUtleTAxMjM0NTY3ODlhYmM=", "-e", "APP_URL=http://127.0.0.1:18086",
			"-e", "DB_CONNECTION=mysql", "-e", "DB_HOST=db", "-e", "DB_DATABASE=snipeit", "-e", "DB_USERNAME=snipeit", "-e", "DB_PASSWORD=" + dbpw,
			"-e", "ADMIN_USER=admin", "-e", "ADMIN_EMAIL=admin@quench-works.com", "-e", "ADMIN_PASSWORD=" + pw}
		start := func(name string) {
			t.Start(name, flags)
			for i := 0; i < 150; i++ {
				if c, _ := restDo("GET", base+"/login", "", "", ""); c == http.StatusOK {
					return
				}
				time.Sleep(2 * time.Second)
			}
			t.Fatalf("snipe-it did not serve /login:\n%s", t.Logs(name))
		}
		name := t.Name("")
		start(name)
		_, page := restDo("GET", base+"/login", "", "", "")
		t.Contains(page, "Snipe-IT", "the login page")
		if got := snipeLogin(base, "admin", "wrong-password-1"); !strings.HasSuffix(got, "/login") {
			t.Fatalf("a wrong password was not sent back to /login: %q", got)
		}
		if got := snipeLogin(base, "admin", pw); got == "" || strings.HasSuffix(got, "/login") {
			t.Fatalf("admin sign-in failed: %q\n%s", got, t.Logs(name))
		}
		docker("rm", "-f", name)
		start(t.Name("again"))
		if got := snipeLogin(base, "admin", pw); got == "" || strings.HasSuffix(got, "/login") {
			t.Fatalf("admin sign-in after a restart failed: %q", got)
		}
		t.Logf("smoke test passed (snipe-it %s: read-only boot against MariaDB, migrations and Passport keys, admin and settings created so setup is done, admin signs in, wrong password refused, account kept across a restart; user %s)", t.Want, u)
	})
}
