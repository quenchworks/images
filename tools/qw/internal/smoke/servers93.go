package smoke

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

func init() {
	// On a read-only rootfs with SQLite on a volume: /health, a superuser from createsuperuser, a
	// password login through the CSRF-protected form, a check pinged over HTTP and read back as up
	// (sendalerts runs beside gunicorn), and the check still up after a restart on the same volume.
	register("healthchecks", func(t *T) {
		const base = "http://127.0.0.1:18000"
		const pw = "Smoke-Pass-2026"
		u := t.User("1001")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		// an empty named volume is seeded from the image's /data, which apko makes 1001-owned
		start := func(name string) {
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/data", "-p", "127.0.0.1:18000:8000",
				"-e", "SECRET_KEY=smoke-secret-key-0123456789abcdef", "-e", "SITE_ROOT=" + base, "-e", "GUNICORN_WORKERS=2"})
			t.waitFor(90, time.Second, name, "healthchecks never answered /health", func() bool {
				if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
					t.Fatalf("healthchecks exited\n%s", t.Logs(name))
				}
				c, _ := Get(base + "/health")
				return c == 200
			})
		}
		shell := func(name, code string) string {
			return t.Docker("exec", name, "python", "manage.py", "shell", "-c", code)
		}
		name := t.Name("")
		start(name)
		t.Docker("exec", name, "python", "manage.py", "createsuperuser", "--email", "smoke@example.com", "--password", pw)

		jar, _ := cookiejar.New(nil)
		cl := &http.Client{Jar: jar, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		resp, err := cl.Get(base + "/accounts/login/")
		if err != nil {
			t.Fatalf("login page: %v", err)
		}
		page, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		token := t.Match(string(page), `name="csrfmiddlewaretoken" value="([^"]+)"`, "CSRF token on the login page")
		login := func(p string) int {
			req, _ := http.NewRequest("POST", base+"/accounts/login/", strings.NewReader(url.Values{
				"csrfmiddlewaretoken": {token}, "action": {"login"}, "email": {"smoke@example.com"}, "password": {p}}.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Referer", base+"/accounts/login/")
			resp, err := cl.Do(req)
			if err != nil {
				t.Fatalf("login: %v", err)
			}
			resp.Body.Close()
			return resp.StatusCode
		}
		// a failed login renders the form again (200); a good one redirects
		if c := login("wrong-password-1"); c != 200 {
			t.Fatalf("a wrong password answered %d, expected the form again", c)
		}
		if c := login(pw); c != 302 {
			t.Fatalf("login answered %d, expected a redirect\n%s", c, t.Logs(name))
		}
		resp, err = cl.Get(base + "/accounts/profile/")
		if err != nil || resp.StatusCode != 200 {
			t.Fatalf("profile page after login: %v %v", err, resp)
		}
		resp.Body.Close()

		code := strings.TrimSpace(shell(name, "from hc.api.models import Check; from hc.accounts.models import Project; "+
			"print(Check.objects.create(project=Project.objects.get(owner__email='smoke@example.com'), name='smoke').code)"))
		code = t.Match(code, `([0-9a-f-]{36})`, "new check code")
		if c, b := Get(base + "/ping/" + code); c != 200 || !strings.Contains(b, "OK") {
			t.Fatalf("ping answered %d %q", c, b)
		}
		status := func(n string) string {
			return strings.TrimSpace(shell(n, "from hc.api.models import Check; print(Check.objects.get(code='"+code+"').status)"))
		}
		t.Contains(status(name), "up", "check status after its ping")
		docker("rm", "-f", name)
		again := t.Name("again")
		start(again)
		t.Contains(status(again), "up", "check status after a restart on the same volume")
		t.Logf("smoke test passed (healthchecks %s: read-only boot, superuser, form login, ping marks the check up, kept across a restart; user %s)", t.Want, u)
	})
}
