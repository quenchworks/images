package smoke

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

// wallosForm posts a form with c (its cookie jar keeps the PHP session) and returns the status and Location.
func wallosForm(c *http.Client, u string, v url.Values) (int, string) {
	r, err := c.PostForm(u, v)
	if err != nil {
		return 0, err.Error()
	}
	defer r.Body.Close()
	io.Copy(io.Discard, r.Body)
	return r.StatusCode, r.Header.Get("Location")
}

func init() {
	// On a read-only rootfs with /var/lib/wallos on a volume: the database is created, WALLOS_ADMIN_* registers
	// the first account through upstream's registration.php, web registration is then closed, a wrong password is refused, the account signs in and sees the
	// dashboard, the database is not served, the schedule runs, and the account survives a restart.
	register("wallos", func(t *T) {
		const base = "http://127.0.0.1:18093"
		const pw = "Smoke-Wallos-2026!"
		u := t.User("1001")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		client := func() *http.Client {
			jar, _ := cookiejar.New(nil)
			return &http.Client{Jar: jar, Timeout: 15 * time.Second,
				CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		}
		start := func(name string) {
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/var/lib/wallos", "-p", "127.0.0.1:18093:8080",
				"-e", "WALLOS_ADMIN_USERNAME=admin", "-e", "WALLOS_ADMIN_PASSWORD=" + pw, "-e", "WALLOS_ADMIN_EMAIL=admin@quench-works.com"})
			t.WaitHTTP(base+"/health.php", 90, name)
		}
		login := func(pass string) (int, string, *http.Client) {
			c := client()
			code, loc := wallosForm(c, base+"/login.php", url.Values{"username": {"admin"}, "password": {pass}})
			return code, loc, c
		}
		name := t.Name("")
		start(name)
		if c, _ := Get(base + "/db/wallos.db"); c != http.StatusForbidden {
			t.Fatalf("/db/wallos.db answered %d, want 403", c)
		}
		t.WaitLog(name, "wallos-admin: registered admin", 10)
		reg := url.Values{"username": {"second"}, "firstname": {"Smoke"}, "lastname": {"Test"}, "email": {"second@quench-works.com"},
			"password": {pw}, "confirm_password": {pw}, "main_currency": {"EUR"}, "language": {"en"}}
		if code, loc := wallosForm(client(), base+"/registration.php", reg); code != http.StatusFound || loc != "login.php" {
			t.Fatalf("a second registration was not sent to login.php: %d %q", code, loc)
		}
		if code, loc, _ := login("wrong-password-1"); code == http.StatusFound && loc == "." {
			t.Fatalf("a wrong password signed in")
		}
		code, loc, c := login(pw)
		if code != http.StatusFound || loc != "." {
			t.Fatalf("sign-in: %d %q\n%s", code, loc, t.Logs(name))
		}
		if r, err := client().Get(base + "/subscriptions.php"); err != nil || r.StatusCode != http.StatusFound {
			t.Fatalf("subscriptions.php without a session was not redirected: %v", err)
		}
		r, err := c.Get(base + "/subscriptions.php")
		if err != nil {
			t.Fatalf("subscriptions.php: %v", err)
		}
		b, _ := io.ReadAll(r.Body)
		r.Body.Close()
		if r.StatusCode != http.StatusOK || !strings.Contains(string(b), "subscriptions") {
			t.Fatalf("subscriptions.php: %d", r.StatusCode)
		}
		t.WaitLog(name, "schedule entered RUNNING state", 20)
		docker("rm", "-f", name)
		start(t.Name("again"))
		if code, loc, _ := login(pw); code != http.StatusFound || loc != "." {
			t.Fatalf("sign-in after a restart: %d %q", code, loc)
		}
		t.Logf("smoke test passed (wallos %s: read-only boot, database created, admin from the environment and registration closed, wrong password refused, subscriptions page behind the session, database not served, schedule running, account kept across a restart; user %s)", t.Want, u)
	})
}
