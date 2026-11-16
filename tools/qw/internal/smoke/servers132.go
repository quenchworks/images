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

var fireflyToken = regexp.MustCompile(`name="_token" value="([^"]+)"`)

// fireflyForm loads a page for its CSRF token, then posts the form with a fresh cookie jar. It returns the
// status and the Location of the POST (redirects are not followed).
func fireflyForm(base, page string, fields url.Values) (int, string, string) {
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	r, err := c.Get(base + page)
	if err != nil {
		return 0, "", err.Error()
	}
	b, _ := io.ReadAll(r.Body)
	r.Body.Close()
	m := fireflyToken.FindStringSubmatch(string(b))
	if m == nil {
		return r.StatusCode, "", "no _token on " + page
	}
	fields.Set("_token", m[1])
	r, err = c.PostForm(base+page, fields)
	if err != nil {
		return 0, "", err.Error()
	}
	defer r.Body.Close()
	return r.StatusCode, r.Header.Get("Location"), ""
}

func init() {
	// On a read-only rootfs with storage on a volume: the first user registers through the web form, a
	// wrong password is refused, the right one signs in, and it still does after a restart.
	register("firefly-iii", func(t *T) {
		const base = "http://127.0.0.1:18084"
		const email = "smoke@quench-works.com"
		const pw = "Smoke-Pass-2026-firefly"
		u := t.User("1001")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		flags := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/var/lib/firefly-iii",
			"-e", "APP_KEY=QuenchSmokeKey0123456789abcdefgh", "-e", "APP_URL=http://127.0.0.1:18084", "-p", "127.0.0.1:18084:8080"}
		start := func(name string) {
			t.Start(name, flags)
			for i := 0; i < 120; i++ {
				if c, _ := restDo("GET", base+"/login", "", "", ""); c == http.StatusOK {
					return
				}
				time.Sleep(time.Second)
			}
			t.Fatalf("firefly-iii did not serve /login:\n%s", t.Logs(name))
		}
		login := func(pass string) string {
			_, loc, e := fireflyForm(base, "/login", url.Values{"email": {email}, "password": {pass}})
			return loc + e
		}
		name := t.Name("")
		start(name)
		_, page := restDo("GET", base+"/login", "", "", "")
		t.Contains(page, "Firefly III", "the login page")
		code, loc, e := fireflyForm(base, "/register", url.Values{"email": {email}, "password": {pw}, "password_confirmation": {pw}})
		if code != http.StatusFound || strings.Contains(loc, "register") || e != "" {
			t.Fatalf("registration: %d %q %s\n%s", code, loc, e, t.Logs(name))
		}
		if got := login("wrong-password-1"); !strings.HasSuffix(got, "/login") {
			t.Fatalf("a wrong password was not sent back to /login: %q", got)
		}
		if got := login(pw); strings.HasSuffix(got, "/login") || got == "" {
			t.Fatalf("sign-in failed: %q\n%s", got, t.Logs(name))
		}
		docker("rm", "-f", name)
		start(t.Name("again"))
		if got := login(pw); strings.HasSuffix(got, "/login") || got == "" {
			t.Fatalf("sign-in after a restart failed: %q", got)
		}
		t.Logf("smoke test passed (firefly-iii %s: read-only boot, migrations and Passport keys on the volume, first user registered, wrong password refused, sign-in before and after a restart; user %s)", t.Want, u)
	})
}
