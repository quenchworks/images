package smoke

import (
	"strings"
	"time"
)

func init() {
	// On a read-only rootfs with /cloudreve/data on a volume: the web UI is served, the first account
	// registers (and becomes the administrator), logs in for a token, a wrong password is refused, and
	// the account still logs in after a restart on the same volume.
	register("cloudreve", func(t *T) {
		const base = "http://127.0.0.1:15212"
		const pw = "Smoke-Admin-2026"
		u := t.User("1001")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		json := map[string]string{"Content-Type": "application/json"}
		start := func(name string) {
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/cloudreve/data", "-p", "127.0.0.1:15212:5212"})
			t.waitFor(60, time.Second, name, "cloudreve never answered ping", func() bool {
				if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
					t.Fatalf("cloudreve exited\n%s", t.Logs(name))
				}
				c, b := Get(base + "/api/v4/site/ping")
				return c == 200 && strings.Contains(b, t.Want)
			})
		}
		login := func(p string) string {
			_, b := DoH("POST", base+"/api/v4/session/token", `{"email":"admin@quench.test","password":"`+p+`"}`, json)
			return b
		}
		name := t.Name("")
		start(name)
		if c, b := Get(base + "/"); c != 200 || !strings.Contains(b, "<html") {
			t.Fatalf("the web UI answered %d", c)
		}
		if _, b := DoH("POST", base+"/api/v4/user", `{"email":"admin@quench.test","password":"`+pw+`","language":"en-US"}`, json); !strings.Contains(b, `"code":0`) {
			t.Fatalf("registering the first account failed: %s\n%s", b, t.Logs(name))
		}
		if b := login(pw); !strings.Contains(b, `"code":0`) || !strings.Contains(b, "access_token") {
			t.Fatalf("login failed: %s", b)
		}
		if b := login("wrong-password-1"); strings.Contains(b, `"code":0`) {
			t.Fatalf("a wrong password logged in: %s", b)
		}
		docker("rm", "-f", name)
		again := t.Name("again")
		start(again)
		if b := login(pw); !strings.Contains(b, `"code":0`) {
			t.Fatalf("the account is gone after a restart: %s", b)
		}
		t.Logf("smoke test passed (cloudreve %s: read-only boot, UI served, first account registered and logged in, wrong password refused, kept across a restart; user %s)", t.Want, u)
	})
}
