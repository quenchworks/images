package smoke

import (
	"context"
	"net"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func init() {
	// On a read-only rootfs with a bcrypt user from TINYAUTH_AUTH_USERS: /api/healthz, a wrong
	// password refused, a login, and the nginx forward-auth endpoint answering 401 without the
	// session cookie and 200 with it (the check an ingress makes before every request).
	register("tinyauth", func(t *T) {
		// Tinyauth derives the cookie domain from TINYAUTH_APPURL, which must be sub.domain.tld;
		// every request dials the published port whatever the host name
		const base = "http://auth.quench.test:13300"
		const pw = "Smoke-Pass-2026"
		u := t.User("1001")
		t.Contains(t.RunAny("version"), t.Want, "tinyauth version")
		hash, _ := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--tmpfs", "/data:rw,uid=1001,gid=1001", "-p", "127.0.0.1:13300:3000",
			"-e", "TINYAUTH_APPURL=" + base, "-e", "TINYAUTH_AUTH_USERS=smoke:" + string(hash)})
		t.waitFor(60, time.Second, name, "tinyauth never answered /api/healthz", func() bool {
			if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
				t.Fatalf("tinyauth exited\n%s", t.Logs(name))
			}
			c, _ := Get("http://127.0.0.1:13300/api/healthz")
			return c == 200
		})
		tr := &http.Transport{DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, "127.0.0.1:13300")
		}}
		jar, _ := cookiejar.New(nil)
		cl := &http.Client{Jar: jar, Transport: tr, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		login := func(p string) int {
			resp, err := cl.Post(base+"/api/user/login", "application/json", strings.NewReader(`{"username":"smoke","password":"`+p+`"}`))
			if err != nil {
				t.Fatalf("login: %v", err)
			}
			resp.Body.Close()
			return resp.StatusCode
		}
		check := func(c *http.Client) int {
			req, _ := http.NewRequest("GET", base+"/api/auth/nginx", nil)
			// nginx auth_request sends only X-Original-URL; mixing in X-Forwarded-* is a 400
			req.Header.Set("X-Original-URL", "http://app.quench.test/")
			resp, err := c.Do(req)
			if err != nil {
				t.Fatalf("forward-auth check: %v", err)
			}
			resp.Body.Close()
			return resp.StatusCode
		}
		if c := check(&http.Client{Transport: tr, Timeout: 10 * time.Second}); c != 401 {
			t.Fatalf("forward-auth without a session answered %d, expected 401\n%s", c, t.Logs(name))
		}
		if c := login("wrong-password-1"); c == 200 {
			t.Fatalf("a wrong password logged in")
		}
		if c := login(pw); c != 200 {
			t.Fatalf("login answered %d\n%s", c, t.Logs(name))
		}
		if c := check(cl); c != 200 {
			t.Fatalf("forward-auth with the session answered %d, expected 200\n%s", c, t.Logs(name))
		}
		t.Logf("smoke test passed (tinyauth %s: read-only boot, wrong password refused, login, forward-auth 401 without and 200 with the session; user %s)", t.Want, u)
	})
}
