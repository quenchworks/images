package smoke

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func init() {
	// On a read-only rootfs with /var/lib/traggo on a volume and the first admin from the environment: the web UI is
	// served, a wrong password gets no token, the token creates a tag that the API lists, an unauthenticated tag query
	// is refused, and the account and the tag survive a restart.
	register("traggo", func(t *T) {
		const base = "http://127.0.0.1:18105"
		const pw = "Smoke-Traggo-2026!"
		u := t.User("1001")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		start := func(name string) {
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/var/lib/traggo", "-p", "127.0.0.1:18105:3030",
				"-e", "TRAGGO_DEFAULT_USER_NAME=smoke", "-e", "TRAGGO_DEFAULT_USER_PASS=" + pw})
			t.WaitHTTP(base+"/", 60, name)
		}
		gql := func(token, query string) (int, string) {
			b, _ := json.Marshal(map[string]string{"query": query})
			h := map[string]string{"Content-Type": "application/json"}
			if token != "" {
				h["Authorization"] = "traggo " + token
			}
			return DoH("POST", base+"/graphql", string(b), h)
		}
		login := func(pass string) string {
			_, b := gql("", `mutation { login(username: "smoke", pass: "`+pass+`", deviceName: "smoke", type: NoExpiry, cookie: false) { token } }`)
			var r struct {
				Data struct{ Login *struct{ Token string } }
			}
			json.Unmarshal([]byte(b), &r)
			if r.Data.Login == nil {
				return ""
			}
			return r.Data.Login.Token
		}
		tagged := func(tok string) bool {
			c, b := gql(tok, `{ tags { key } }`)
			return c == http.StatusOK && strings.Contains(b, `"quench-smoke"`)
		}
		name := t.Name("")
		start(name)
		_, page := Get(base + "/")
		t.Contains(page, "<div id=\"root\"", "the web UI")
		if tok := login("wrong-password-1"); tok != "" {
			t.Fatalf("a wrong password got a token")
		}
		tok := login(pw)
		if tok == "" {
			t.Fatalf("login failed\n%s", t.Logs(name))
		}
		if c, b := gql(tok, `mutation { createTag(key: "quench-smoke", color: "#00aa88") { key } }`); c != http.StatusOK || strings.Contains(b, `"errors"`) {
			t.Fatalf("createTag: %d %s", c, b)
		}
		if !tagged(tok) {
			t.Fatalf("the tag is not listed")
		}
		if _, b := gql("", `{ tags { key } }`); strings.Contains(b, `"quench-smoke"`) {
			t.Fatalf("an unauthenticated query listed tags")
		}
		docker("rm", "-f", name)
		time.Sleep(time.Second)
		start(t.Name("again"))
		if tok := login(pw); tok == "" || !tagged(tok) {
			t.Fatalf("after a restart: login or tag missing")
		}
		t.Logf("smoke test passed (traggo %s: read-only boot, web UI, wrong password refused, tag created and listed, unauthenticated query refused, kept across a restart; user %s)", t.Want, u)
	})
}
