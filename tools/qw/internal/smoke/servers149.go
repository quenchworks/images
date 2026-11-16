package smoke

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func init() {
	// On a read-only rootfs with /data on a volume and password auth: /health answers, the UI is served, a wrong
	// password gets no token, the token creates a note that full-text search finds, and the note survives a restart.
	register("flatnotes", func(t *T) {
		const base = "http://127.0.0.1:18102"
		const pw = "Smoke-Flatnotes-2026!"
		u := t.User("1001")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		start := func(name string) {
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/data", "-p", "127.0.0.1:18102:8080",
				"-e", "FLATNOTES_AUTH_TYPE=password", "-e", "FLATNOTES_USERNAME=smoke", "-e", "FLATNOTES_PASSWORD=" + pw,
				"-e", "FLATNOTES_SECRET_KEY=quench-smoke-secret-key-0123456789abcdef"})
			t.WaitHTTP(base+"/health", 60, name)
		}
		token := func(pass string) (int, string) {
			c, b := DoH("POST", base+"/api/token", `{"username":"smoke","password":"`+pass+`"}`, map[string]string{"Content-Type": "application/json"})
			var r struct {
				AccessToken string `json:"access_token"`
			}
			json.Unmarshal([]byte(b), &r)
			return c, r.AccessToken
		}
		auth := func(tok string) map[string]string {
			return map[string]string{"Authorization": "Bearer " + tok, "Content-Type": "application/json"}
		}
		name := t.Name("")
		start(name)
		_, page := Get(base + "/")
		t.Contains(page, "<base href=\"/\"", "the web UI")
		if c, tok := token("wrong-password-1"); c/100 == 2 || tok != "" {
			t.Fatalf("a wrong password got %d", c)
		}
		if c, _ := DoH("GET", base+"/api/notes/anything", "", nil); c != http.StatusUnauthorized {
			t.Fatalf("an anonymous read answered %d, want 401", c)
		}
		c, tok := token(pw)
		if c != http.StatusOK || tok == "" {
			t.Fatalf("login: %d\n%s", c, t.Logs(name))
		}
		if c, b := DoH("POST", base+"/api/notes", `{"title":"Quench smoke","content":"the quokkasmoke word is searchable"}`, auth(tok)); c/100 != 2 {
			t.Fatalf("create a note: %d %s", c, b)
		}
		found := func(tok string) bool {
			for i := 0; i < 10; i++ {
				if c, b := DoH("GET", base+"/api/search?term=quokkasmoke", "", auth(tok)); c == http.StatusOK && strings.Contains(b, "Quench smoke") {
					return true
				}
				time.Sleep(time.Second)
			}
			return false
		}
		if !found(tok) {
			t.Fatalf("search did not find the note")
		}
		docker("rm", "-f", name)
		start(t.Name("again"))
		_, tok = token(pw)
		if c, b := DoH("GET", base+"/api/notes/Quench%20smoke", "", auth(tok)); c != http.StatusOK || !strings.Contains(b, "quokkasmoke") {
			t.Fatalf("read the note after a restart: %d %s", c, b)
		}
		if !found(tok) {
			t.Fatalf("search did not find the note after a restart")
		}
		t.Logf("smoke test passed (flatnotes %s: read-only boot, UI, wrong password and anonymous read refused, note created and found by search, kept across a restart; user %s)", t.Want, u)
	})
}
