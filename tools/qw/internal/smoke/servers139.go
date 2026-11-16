package smoke

import (
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var gravAccessToken = regexp.MustCompile(`"access_token"\s*:\s*"([^"]+)"`)

func gravDo(method, url, bearer, body string) (int, string) {
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	r, err := noRedirect.Do(req)
	if err != nil {
		return 0, err.Error()
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return r.StatusCode, string(b)
}

func init() {
	// On a read-only rootfs with /var/lib/grav on a volume: the site renders, the API refuses a wrong
	// password and gives the admin from GRAV_ADMIN_* a token that reads the pages, user config is not
	// served, and the account survives a restart.
	register("grav", func(t *T) {
		const base = "http://127.0.0.1:18091"
		const pw = "Smoke-Grav-2026x"
		u := t.User("1001")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		flags := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/var/lib/grav", "-p", "127.0.0.1:18091:8080",
			"-e", "GRAV_ADMIN_USER=admin", "-e", "GRAV_ADMIN_PASSWORD=" + pw, "-e", "GRAV_ADMIN_EMAIL=admin@quench-works.com"}
		start := func(name string) {
			t.Start(name, flags)
			for i := 0; i < 90; i++ {
				if c, _ := restDo("GET", base+"/", "", "", ""); c == http.StatusOK {
					return
				}
				time.Sleep(time.Second)
			}
			t.Fatalf("grav did not serve /:\n%s", t.Logs(name))
		}
		login := func(pass string) (int, string) {
			c, body := gravDo("POST", base+"/api/v1/auth/token", "", `{"username":"admin","password":"`+pass+`"}`)
			m := gravAccessToken.FindStringSubmatch(body)
			if m == nil {
				return c, ""
			}
			return c, m[1]
		}
		name := t.Name("")
		start(name)
		_, page := restDo("GET", base+"/", "", "", "")
		t.Contains(page, "Grav", "the home page")
		if c, _ := restDo("GET", base+"/user/config/system.yaml", "", "", ""); c != http.StatusForbidden {
			t.Fatalf("/user/config/system.yaml answered %d, want 403", c)
		}
		if c, tok := login("wrong-password-1"); c < 400 || tok != "" {
			t.Fatalf("a wrong password got %d", c)
		}
		c, tok := login(pw)
		if c != http.StatusOK || tok == "" {
			t.Fatalf("admin token: %d\n%s", c, t.Logs(name))
		}
		if c, body := gravDo("GET", base+"/api/v1/pages", tok, ""); c != http.StatusOK {
			t.Fatalf("GET /api/v1/pages with the token: %d %s", c, body)
		}
		docker("rm", "-f", name)
		start(t.Name("again"))
		if c, tok := login(pw); c != http.StatusOK || tok == "" {
			t.Fatalf("admin token after a restart: %d", c)
		}
		t.Logf("smoke test passed (grav %s: read-only boot, site renders, user config not served, wrong password refused, admin token reads the pages, account kept across a restart; user %s)", t.Want, u)
	})
}
