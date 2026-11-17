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

func init() {
	// On a read-only rootfs with /config on a volume: `cps.py -s admin:<pw>` replaces upstream's admin123, the server
	// starts on 8083, the generated password signs in through the CSRF-protected form while admin123 and a wrong
	// password are refused, and the password survives a restart.
	register("calibre-web", func(t *T) {
		const base = "http://127.0.0.1:18112"
		const pw = "Smoke-CalibreWeb-2026!"
		u := t.User("1001")
		vol := t.Name("config")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		flags := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/config"}
		// the first start creates app.db with admin/admin123; -s then changes the password and exits
		set := t.Name("setpw")
		t.Cleanup(func() { docker("rm", "-f", set) })
		out, err := docker(append(append([]string{"run", "--name", set, "--user", "1001"}, flags...), t.Image, "-p", "/config/app.db", "-s", "admin:"+pw)...)
		if err != nil || !strings.Contains(out, "Password for user 'admin' changed") {
			t.Fatalf("set the admin password: %v\n%s", err, out)
		}
		start := func(name string) {
			t.Start(name, append(append([]string{}, flags...), "-p", "127.0.0.1:18112:8083"))
			t.WaitHTTP(base+"/login", 90, name)
		}
		csrf := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`)
		// login returns whether the session reached the library (a redirect away from /login)
		login := func(pass string) bool {
			jar, _ := cookiejar.New(nil)
			c := &http.Client{Jar: jar, Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
			r, err := c.Get(base + "/login")
			if err != nil {
				return false
			}
			b, _ := io.ReadAll(r.Body)
			r.Body.Close()
			m := csrf.FindSubmatch(b)
			if m == nil {
				t.Fatalf("no csrf_token on /login")
			}
			r, err = c.PostForm(base+"/login", url.Values{"csrf_token": {string(m[1])}, "username": {"admin"}, "password": {pass}, "submit": {"Login"}})
			if err != nil {
				return false
			}
			r.Body.Close()
			return r.StatusCode/100 == 3 && !strings.Contains(r.Header.Get("Location"), "login")
		}
		name := t.Name("")
		start(name)
		if !login(pw) {
			t.Fatalf("the generated password did not sign in\n%s", t.Logs(name))
		}
		if login("admin123") {
			t.Fatalf("upstream's default admin123 still signs in")
		}
		if login("wrong-password-1") {
			t.Fatalf("a wrong password signed in")
		}
		docker("rm", "-f", name)
		start(t.Name("again"))
		if !login(pw) {
			t.Fatalf("the password did not survive a restart")
		}
		t.Logf("smoke test passed (calibre-web %s: read-only boot, admin123 replaced by -s, form sign-in with CSRF, admin123 and a wrong password refused, kept across a restart; user %s)", t.Want, u)
	})
}
