package smoke

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func init() {
	// On a read-only rootfs with /var/lib/donetick on a volume: /health reports the version, the embedded web UI is
	// served, a user signs up, a wrong password is refused, the token creates and lists a chore, and the account and
	// the chore survive a restart (config with its generated JWT secret and SQLite on the volume).
	register("donetick", func(t *T) {
		const base = "http://127.0.0.1:18098"
		const pw = "Smoke-Donetick-2026!"
		u := t.User("1001")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		start := func(name string) {
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/var/lib/donetick", "-p", "127.0.0.1:18098:2021"})
			t.WaitHTTP(base+"/health", 60, name)
		}
		login := func(pass string) (int, string) {
			c, b := DoH("POST", base+"/api/v1/auth/login/legacy", `{"username":"smoke","password":"`+pass+`"}`, map[string]string{"Content-Type": "application/json"})
			var r struct{ Token string }
			json.Unmarshal([]byte(b), &r)
			return c, r.Token
		}
		bearer := func(method, url, tok, body string) (int, string) {
			return DoH(method, url, body, map[string]string{"Authorization": "Bearer " + tok, "Content-Type": "application/json"})
		}
		name := t.Name("")
		start(name)
		_, h := Get(base + "/health")
		if t.Want != "" {
			t.Contains(h, `"version":"`+t.Want+`"`, "/health")
		}
		_, page := Get(base + "/")
		t.Contains(page, `<div id="root"`, "the embedded web UI")
		if c, b := DoH("POST", base+"/api/v1/auth/", `{"username":"smoke","password":"`+pw+`","email":"smoke@quench-works.com","displayName":"Smoke"}`, map[string]string{"Content-Type": "application/json"}); c/100 != 2 {
			t.Fatalf("sign-up: %d %s\n%s", c, b, t.Logs(name))
		}
		if c, tok := login("wrong-password-1"); c != http.StatusUnauthorized || tok != "" {
			t.Fatalf("a wrong password got %d", c)
		}
		c, tok := login(pw)
		if c != http.StatusOK || tok == "" {
			t.Fatalf("login: %d", c)
		}
		if c, b := bearer("POST", base+"/api/v1/chores/", tok, `{"name":"Quench smoke chore","frequencyType":"once","assignStrategy":"no_assignee","assignees":[]}`); c/100 != 2 {
			t.Fatalf("create chore: %d %s", c, b)
		}
		listed := func(tok string) bool {
			c, b := bearer("GET", base+"/api/v1/chores/", tok, "")
			return c == http.StatusOK && strings.Contains(b, "Quench smoke chore")
		}
		if !listed(tok) {
			t.Fatalf("the chore is not listed")
		}
		docker("rm", "-f", name)
		time.Sleep(time.Second)
		start(t.Name("again"))
		if c, tok := login(pw); c != http.StatusOK || !listed(tok) {
			t.Fatalf("after a restart: login %d, chore listed %v", c, listed(tok))
		}
		t.Logf("smoke test passed (donetick %s: read-only boot, health and web UI, sign-up, wrong password refused, chore created and listed, account and chore kept across a restart; user %s)", t.Want, u)
	})
}
