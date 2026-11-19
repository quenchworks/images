package smoke

import (
	"net/http"
	"strings"
	"time"
)

func init() {
	// On a read-only rootfs with /app/user-data on a volume and basic auth from the environment: the app shell is
	// served, the config-save API refuses no credentials and a wrong password, the admin saves a new conf.yml (the
	// old one is backed up on the volume), and after a restart the saved config is still served.
	register("dashy", func(t *T) {
		const base = "http://127.0.0.1:18092"
		const user, pw = "admin", "Smoke-Dashy-2026"
		u := t.User("1001")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		flags := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/app/user-data", "-p", "127.0.0.1:18092:8080",
			"-e", "BASIC_AUTH_USERNAME=" + user, "-e", "BASIC_AUTH_PASSWORD=" + pw}
		start := func(name string) {
			t.Start(name, flags)
			for i := 0; i < 60; i++ {
				if c, _ := restDo("GET", base+"/healthz", "", "", ""); c == http.StatusOK {
					return
				}
				time.Sleep(2 * time.Second)
			}
			t.Fatalf("dashy did not answer /healthz:\n%s", t.Logs(name))
		}
		save := func(u, p, body string) (int, string) {
			req, _ := http.NewRequest("POST", base+"/config-manager/save", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			if u != "" {
				req.SetBasicAuth(u, p)
			}
			return insecureDo(req)
		}
		name := t.Name("")
		start(name)
		_, page := restDo("GET", base+"/", "", "", "")
		t.ContainsFold(page, "dashy", "the app shell")
		const conf = `{"config":"pageInfo:\n  title: Quench Smoke\nsections:\n- name: Links\n  items:\n  - title: QuenchWorks\n    url: https://quench-works.com\n","filename":"conf.yml"}`
		if c, _ := save("", "", conf); c != http.StatusUnauthorized {
			t.Fatalf("config save without credentials got %d, want 401", c)
		}
		if c, _ := save(user, "wrong-password-1", conf); c != http.StatusUnauthorized {
			t.Fatalf("config save with a wrong password got %d, want 401", c)
		}
		if c, b := save(user, pw, conf); c != http.StatusOK || !strings.Contains(b, `"success":true`) {
			t.Fatalf("config save as admin: %d %s\n%s", c, b, t.Logs(name))
		}
		docker("rm", "-f", name)
		start(t.Name("again"))
		_, got := restDo("GET", base+"/conf.yml", user, pw, "")
		t.Contains(got, "Quench Smoke", "conf.yml after a restart")
		t.Logf("smoke test passed (dashy %s: read-only boot, app shell served, config save refuses no credentials and a wrong password, admin saves conf.yml, kept across a restart; user %s)", t.Want, u)
	})
}
