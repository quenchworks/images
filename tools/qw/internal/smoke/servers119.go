package smoke

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"time"
)

// readeckLogin posts the web login form and returns the status and, on success, whether the session
// cookie then opens /bookmarks.
func readeckLogin(base, user, pass string) (int, bool) {
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar, Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	r, err := c.PostForm(base+"/login", url.Values{"username": {user}, "password": {pass}})
	if err != nil {
		return 0, false
	}
	r.Body.Close()
	if r.StatusCode != http.StatusSeeOther {
		return r.StatusCode, false
	}
	b, err := c.Get(base + "/bookmarks")
	if err != nil {
		return r.StatusCode, false
	}
	b.Body.Close()
	return r.StatusCode, b.StatusCode == http.StatusOK
}

func init() {
	// On a read-only rootfs with /readeck on a volume: `readeck user` creates an admin, the web login
	// refuses a wrong password and opens a session with the right one, and the user is still there after
	// a restart.
	register("readeck", func(t *T) {
		const base = "http://127.0.0.1:18002"
		const pw = "Smoke-Pass-2026"
		u := t.User("1001")
		t.Contains(t.RunAny("version"), t.Want, "readeck version")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		flags := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/readeck"}
		if out, rc := t.RunAnyFor(2*time.Minute, flags, "user", "-config", "/readeck/config.toml", "-u", "smoke", "-p", pw, "-email", "smoke@quench.test", "-group", "admin"); rc != 0 {
			t.Fatalf("readeck user failed (%d):\n%s", rc, out)
		}
		start := func(name string) {
			t.Start(name, append(flags, "-p", "127.0.0.1:18002:8000"))
			t.WaitHTTP(base+"/login", 60, name)
		}
		name := t.Name("")
		start(name)
		if code, _ := readeckLogin(base, "smoke", "wrong-password-1"); code == http.StatusSeeOther {
			t.Fatalf("a wrong password signed in")
		}
		docker("rm", "-f", name)
		start(t.Name("again"))
		if code, ok := readeckLogin(base, "smoke", pw); !ok {
			t.Fatalf("sign-in after a restart failed (%d); the user is gone?", code)
		}
		t.Logf("smoke test passed (readeck %s: read-only boot, admin created by readeck user, wrong password refused, sign-in and a session after a restart; user %s)", t.Want, u)
	})
}
