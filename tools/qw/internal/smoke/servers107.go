package smoke

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"
)

func init() {
	// On a read-only rootfs with SQLite on a volume: `semaphore user add --admin` creates the first admin,
	// the server serves its UI, the admin logs in and reads /api/user, a wrong password is refused, the
	// admin survives a restart, and the bundled Ansible runs.
	register("semaphore", func(t *T) {
		const base = "http://127.0.0.1:13000"
		const pw = "Smoke-Admin-2026"
		u := t.User("1001")
		t.Contains(t.RunAny("version"), t.Want, "semaphore version")
		if out, err := t.RunFlags([]string{"--entrypoint", "/opt/ansible/venv/bin/ansible", "--read-only", "--tmpfs", "/tmp:rw,mode=1777"}, "--version"); err != nil || !strings.Contains(out, "ansible") {
			t.Fatalf("the bundled ansible did not run: %v\n%s", err, out)
		}
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		key := func() string { b := make([]byte, 32); _, _ = rand.Read(b); return base64.StdEncoding.EncodeToString(b) }
		// session cookies and stored access keys need these; the chart keeps them in a Secret
		ro := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/var/lib/semaphore",
			"-e", "SEMAPHORE_COOKIE_HASH=" + key(), "-e", "SEMAPHORE_COOKIE_ENCRYPTION=" + key(), "-e", "SEMAPHORE_ACCESS_KEY_ENCRYPTION=" + key()}
		if out, err := t.RunFlags(ro, "user", "add", "--no-config", "--admin", "--login", "admin", "--name", "Admin", "--email", "admin@quench.test", "--password", pw); err != nil {
			t.Fatalf("user add: %v\n%s", err, out)
		}
		login := func(p string) (int, *http.Client) {
			jar, _ := cookiejar.New(nil)
			cl := &http.Client{Jar: jar, Timeout: 10 * time.Second}
			r, err := cl.Post(base+"/api/auth/login", "application/json", strings.NewReader(`{"auth":"admin","password":"`+p+`"}`))
			if err != nil {
				return 0, cl
			}
			r.Body.Close()
			return r.StatusCode, cl
		}
		start := func(name string) {
			t.Start(name, append(ro, "-p", "127.0.0.1:13000:3000"))
			t.waitFor(60, time.Second, name, "semaphore never served its UI", func() bool {
				if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
					t.Fatalf("semaphore exited\n%s", t.Logs(name))
				}
				c, _ := Get(base + "/api/ping")
				return c == 200
			})
		}
		check := func(name string) {
			c, cl := login(pw)
			if c != 204 && c != 200 {
				t.Fatalf("admin login answered %d\n%s", c, t.Logs(name))
			}
			r, err := cl.Get(base + "/api/user")
			if err != nil || r.StatusCode != 200 {
				t.Fatalf("/api/user after login: %v %v", err, r)
			}
			r.Body.Close()
		}
		name := t.Name("")
		start(name)
		if c, b := Get(base + "/"); c != 200 || !strings.Contains(b, "<html") {
			t.Fatalf("the web UI answered %d", c)
		}
		check(name)
		if c, _ := login("wrong-password-1"); c != 401 {
			t.Fatalf("a wrong password answered %d, expected 401", c)
		}
		docker("rm", "-f", name)
		again := t.Name("again")
		start(again)
		check(again)
		t.Logf("smoke test passed (semaphore %s: read-only boot, admin created, UI served, login and /api/user, wrong password refused, kept across a restart, ansible runs; user %s)", t.Want, u)
	})
}
