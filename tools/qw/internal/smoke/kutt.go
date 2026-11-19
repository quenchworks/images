package smoke

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// kuttDo sends one JSON request to Kutt with the session cookie Kutt's API reads its JWT from.
func kuttDo(method, url, token, body string) (int, string, http.Header) {
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		return 0, err.Error(), nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if token != "" {
		req.AddCookie(&http.Cookie{Name: "token", Value: token})
	}
	c := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	r, err := c.Do(req)
	if err != nil {
		return 0, err.Error(), nil
	}
	defer r.Body.Close()
	var b strings.Builder
	buf := make([]byte, 32<<10)
	for {
		n, err := r.Body.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return r.StatusCode, b.String(), r.Header
}

func init() {
	// On a read-only rootfs with SQLite on a volume: without JWT_SECRET the launcher refuses to start; with it the
	// migrations run, the first admin is created (a second one is refused), a wrong password is refused, the admin
	// shortens a URL and the short link redirects to it, and after a restart the link still redirects.
	register("kutt", func(t *T) {
		const base = "http://127.0.0.1:18090"
		const email, pw = "admin@quench-works.com", "Smoke-Admin-2026!"
		const target = "https://quench-works.com/smoke-target"
		u := t.User("1001")
		if out, code := t.RunCode([]string{"--read-only"}); code == 0 || !strings.Contains(out, "JWT_SECRET must be set") {
			t.Fatalf("started without JWT_SECRET (exit %d):\n%s", code, out)
		}
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		flags := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/var/lib/kutt", "-p", "127.0.0.1:18090:3000",
			"-e", "JWT_SECRET=quench-kutt-smoke-secret-0123456789", "-e", "DEFAULT_DOMAIN=127.0.0.1:18090", "-e", "TRUST_PROXY=false"}
		start := func(name string) {
			t.Start(name, flags)
			for i := 0; i < 60; i++ {
				if c, _, _ := kuttDo("GET", base+"/api/v2/health", "", ""); c == http.StatusOK {
					return
				}
				time.Sleep(2 * time.Second)
			}
			t.Fatalf("kutt did not answer /api/v2/health:\n%s", t.Logs(name))
		}
		token := func(b string) string {
			var r struct {
				Token string `json:"token"`
			}
			_ = json.Unmarshal([]byte(b), &r)
			return r.Token
		}
		name := t.Name("")
		start(name)
		t.Contains(t.Logs(name), "migrations:", "the launcher's migration line")
		admin := `{"email":"` + email + `","password":"` + pw + `"}`
		c, b, _ := kuttDo("POST", base+"/api/v2/auth/create-admin", "", admin)
		if c != http.StatusCreated || token(b) == "" {
			t.Fatalf("create-admin: %d %s", c, b)
		}
		if c, _, _ := kuttDo("POST", base+"/api/v2/auth/create-admin", "", `{"email":"x@quench-works.com","password":"Second-Admin-2026"}`); c == http.StatusCreated {
			t.Fatalf("a second create-admin was accepted")
		}
		if c, _, _ := kuttDo("POST", base+"/api/v2/auth/login", "", `{"email":"`+email+`","password":"wrong-password-1"}`); c == http.StatusOK {
			t.Fatalf("a wrong password was accepted")
		}
		login := func() string {
			c, b, _ := kuttDo("POST", base+"/api/v2/auth/login", "", admin)
			if c != http.StatusOK || token(b) == "" {
				t.Fatalf("admin login: %d %s", c, b)
			}
			return token(b)
		}
		tok := login()
		c, b, _ = kuttDo("POST", base+"/api/v2/links", tok, `{"target":"`+target+`"}`)
		var link struct {
			Address string `json:"address"`
		}
		if c != http.StatusCreated || json.Unmarshal([]byte(b), &link) != nil || link.Address == "" {
			t.Fatalf("create link: %d %s", c, b)
		}
		follows := func() {
			c, _, h := kuttDo("GET", base+"/"+link.Address, "", "")
			if c/100 != 3 || h.Get("Location") != target {
				t.Fatalf("/%s answered %d to %q, want a redirect to %s", link.Address, c, h.Get("Location"), target)
			}
		}
		follows()
		docker("rm", "-f", name)
		start(t.Name("again"))
		login()
		follows()
		t.Logf("smoke test passed (kutt %s: refuses to start without JWT_SECRET, migrations at start, first admin created and a second refused, wrong password refused, link /%s redirects, kept across a restart; user %s)", t.Want, link.Address, u)
	})
}
