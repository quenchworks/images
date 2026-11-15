package smoke

import (
	"strings"
	"time"
)

func init() {
	// On a read-only rootfs with /ezbookkeeping/data on a volume: the server answers /healthz.json and serves
	// the UI, `userdata user-add` (run in the container) creates a user, the user logs in for a token, a wrong password is
	// refused, and the user still logs in after a restart on the same volume.
	register("ezbookkeeping", func(t *T) {
		const base = "http://127.0.0.1:18089"
		const pw = "Smoke-User-2026"
		u := t.User("1001")
		t.Contains(t.RunAny("--version"), t.Want, "ezbookkeeping --version")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		ro := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/ezbookkeeping/data", "-e", "EBK_SECURITY_SECRET_KEY=smoke-secret-key-0123456789abcdef"}
		json := map[string]string{"Content-Type": "application/json"}
		login := func(p string) string {
			_, b := DoH("POST", base+"/api/authorize.json", `{"loginName":"smoke","password":"`+p+`"}`, json)
			return b
		}
		start := func(name string) {
			t.Start(name, append(ro, "-p", "127.0.0.1:18089:8080"))
			t.waitFor(60, time.Second, name, "ezbookkeeping never answered /healthz.json", func() bool {
				if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
					t.Fatalf("ezbookkeeping exited\n%s", t.Logs(name))
				}
				c, _ := Get(base + "/healthz.json")
				return c == 200
			})
		}
		name := t.Name("")
		start(name)
		// the server creates the schema on start (auto_update_database), then the CLI can add a user
		if out, err := docker("exec", name, "/ezbookkeeping/ezbookkeeping", "userdata", "user-add", "--username", "smoke", "--email", "smoke@quench.test",
			"--nickname", "Smoke", "--password", pw, "--default-currency", "USD"); err != nil {
			t.Fatalf("user-add: %v\n%s", err, out)
		}
		if c, b := Get(base + "/"); c != 200 || !strings.Contains(strings.ToLower(b), "<html") {
			t.Fatalf("the web UI answered %d", c)
		}
		if b := login(pw); !strings.Contains(b, `"token"`) {
			t.Fatalf("login failed: %s\n%s", b, t.Logs(name))
		}
		if b := login("wrong-password-1"); strings.Contains(b, `"token"`) {
			t.Fatalf("a wrong password logged in: %s", b)
		}
		docker("rm", "-f", name)
		again := t.Name("again")
		start(again)
		if b := login(pw); !strings.Contains(b, `"token"`) {
			t.Fatalf("the user is gone after a restart: %s", b)
		}
		t.Logf("smoke test passed (ezbookkeeping %s: read-only boot, user added by the CLI, UI served, login, wrong password refused, kept across a restart; user %s)", t.Want, u)
	})
}
