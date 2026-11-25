package smoke

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"
)

// sb3 smokes SilverBullet 3.x, which dropped SB_USER/SB_AUTH_TOKEN: an empty folder boots an unauthenticated setup
// wizard, /.setup/api/complete creates the admin and the first space, and the server swaps to multi-space in place.
func sb3(t *T) {
	const base = "http://127.0.0.1:18101"
	const page = "# Quench smoke\n\nwritten by the smoke test\n"
	u := t.User("1001")
	vol := t.Name("data")
	t.Docker("volume", "create", vol)
	t.Cleanup(func() { docker("volume", "rm", vol) })
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	do := func(method, path, body string) (int, string) {
		req, _ := http.NewRequest(method, base+path, strings.NewReader(body))
		if strings.HasPrefix(body, "{") {
			req.Header.Set("Content-Type", "application/json")
		}
		r, err := c.Do(req)
		if err != nil {
			return 0, err.Error()
		}
		defer r.Body.Close()
		b, _ := io.ReadAll(r.Body)
		return r.StatusCode, string(b)
	}
	start := func(name string) {
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/data", "-p", "127.0.0.1:18101:3000"})
		t.waitFor(60, time.Second, name, "never listened", func() bool { return strings.Contains(t.Logs(name), "is now running") })
	}
	login := func() {
		if code, b := do("POST", "/.dashboard/api/login", `{"username":"smoke","password":"Smoke-Bullet-2026!"}`); code != 200 || !strings.Contains(b, `"ok"`) {
			t.Fatalf("login: %d %s", code, b)
		}
	}
	read := func() {
		if code, b := do("GET", "/.fs/quench-smoke.md", ""); code != 200 || b != page {
			t.Fatalf("read the page: %d %q", code, b)
		}
	}
	name := t.Name("")
	start(name)
	if code, b := do("POST", "/.setup/api/complete", `{"adminUsername":"smoke","adminPassword":"Smoke-Bullet-2026!","space":{"name":"Notes","prefix":"/"}}`); code != 200 {
		t.Fatalf("setup: %d %s\n%s", code, b, t.Logs(name))
	}
	t.waitFor(60, time.Second, name, "setup did not swap in multi-space", func() bool { return strings.Contains(t.Logs(name), "now serving multi-space") })
	if code, _ := do("POST", "/.setup/api/complete", `{"adminUsername":"evil","adminPassword":"Evil-Bullet-2026!"}`); code/100 == 2 {
		t.Fatalf("setup ran a second time (%d)", code)
	}
	if code, _ := do("PUT", "/.fs/intruder.md", "x"); code/100 == 2 {
		t.Fatalf("an anonymous write answered %d", code)
	}
	login()
	if code, b := do("PUT", "/.fs/quench-smoke.md", page); code/100 != 2 {
		t.Fatalf("write the page: %d %s\n%s", code, b, t.Logs(name))
	}
	read()
	docker("rm", "-f", name)
	time.Sleep(time.Second)
	again := t.Name("again")
	start(again)
	t.Contains(t.Logs(again), "boot mode: multi-space", "restart boot mode")
	read()
	t.Logf("smoke test passed (silverbullet %s: read-only boot, setup API once, anonymous write refused, page written and read after login, kept across a restart; user %s)", t.Want, u)
}
