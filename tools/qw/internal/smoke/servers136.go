package smoke

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

// lycheeSession signs in through the v2 API (session cookie plus the XSRF token Laravel sets) and returns
// the login status and body (204 on success).
func lycheeSession(base, user, pass string) (int, string) {
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar, Timeout: 30 * time.Second}
	r, err := c.Get(base + "/")
	if err != nil {
		return 0, err.Error()
	}
	r.Body.Close()
	u, _ := url.Parse(base)
	xsrf := ""
	for _, ck := range jar.Cookies(u) {
		if ck.Name == "XSRF-TOKEN" {
			xsrf, _ = url.QueryUnescape(ck.Value)
		}
	}
	b, _ := json.Marshal(map[string]string{"username": user, "password": pass})
	do := func(method, path, body string) (int, string) {
		req, _ := http.NewRequest(method, base+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-XSRF-TOKEN", xsrf)
		resp, err := c.Do(req)
		if err != nil {
			return 0, err.Error()
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(raw)
	}
	return do("POST", "/api/v2/Auth::login", string(b))
}

func init() {
	// On a read-only rootfs with /var/lib/lychee on a volume (SQLite): migrations run, the admin comes from
	// ADMIN_USER/ADMIN_PASSWORD, the API refuses a wrong password and signs the admin in (204), and the
	// account survives a restart.
	register("lychee", func(t *T) {
		const base = "http://127.0.0.1:18088"
		const pw = "Smoke-Lychee-2026"
		u := t.User("1001")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		flags := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/var/lib/lychee", "-p", "127.0.0.1:18088:8080",
			"-e", "APP_KEY=base64:UXVlbmNoTHljaGVlU21va2VLZXkwMTIzNDU2Nzg5YWI=", "-e", "APP_URL=http://127.0.0.1:18088",
			"-e", "ADMIN_USER=admin", "-e", "ADMIN_PASSWORD=" + pw}
		start := func(name string) {
			t.Start(name, flags)
			for i := 0; i < 120; i++ {
				if c, _ := restDo("GET", base+"/", "", "", ""); c == http.StatusOK {
					return
				}
				time.Sleep(time.Second)
			}
			t.Fatalf("lychee did not serve /:\n%s", t.Logs(name))
		}
		name := t.Name("")
		start(name)
		if c, _ := lycheeSession(base, "admin", "wrong-password-1"); c < 400 {
			t.Fatalf("a wrong password got %d", c)
		}
		if c, out := lycheeSession(base, "admin", pw); c != http.StatusNoContent {
			t.Fatalf("admin sign-in: %d %q\n%s", c, out, t.Logs(name))
		}
		docker("rm", "-f", name)
		start(t.Name("again"))
		if c, out := lycheeSession(base, "admin", pw); c != http.StatusNoContent {
			t.Fatalf("sign-in after a restart: %d %q", c, out)
		}
		t.Logf("smoke test passed (lychee %s: read-only boot on SQLite, admin from the environment, wrong password refused, admin signs in through the API, account kept across a restart; user %s)", t.Want, u)
	})
}
