package smoke

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func init() {
	// On a read-only rootfs with a home volume (config dir and SQLite there) and a superuser from the environment:
	// the server boots and reports the tag, the React UI is served, auto-login is refused, a wrong password is
	// refused and the right one gets a token, a flow saved through the API reads back, and after a restart on the
	// same volume the flow is still there. Logs must show no Traceback.
	register("langflow", func(t *T) {
		const base = "http://127.0.0.1:18897"
		const pw = "Smoke-Langflow-2026"
		u := t.User("1001")
		vol := t.Name("home")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		flags := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/home/langflow", "-p", "127.0.0.1:18897:7860",
			"-e", "LANGFLOW_SUPERUSER=admin", "-e", "LANGFLOW_SUPERUSER_PASSWORD=" + pw}
		call := func(method, path, token, body string) (int, string) {
			req, _ := http.NewRequest(method, base+path, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			return insecureDo(req)
		}
		login := func(p string) (int, string) {
			req, _ := http.NewRequest("POST", base+"/api/v1/login", strings.NewReader("username=admin&password="+p))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			return insecureDo(req)
		}
		start := func(name string) string {
			t.Start(name, flags)
			t.waitCode(base+"/health_check", 120, 3*time.Second, name, 200)
			if c, _ := call("GET", "/api/v1/auto_login", "", ""); c == 200 {
				t.Fatalf("auto_login answered 200: it must be off")
			}
			if c, _ := login("wrong-password-1"); c != 401 {
				t.Fatalf("login with a wrong password got %d, want 401", c)
			}
			c, b := login(pw)
			var tok struct {
				AccessToken string `json:"access_token"`
			}
			if c != 200 || json.Unmarshal([]byte(b), &tok) != nil || tok.AccessToken == "" {
				t.Fatalf("login: %d %s\n%s", c, b, t.Logs(name))
			}
			return tok.AccessToken
		}
		name := t.Name("")
		tok := start(name)
		_, ver := call("GET", "/api/v1/version", "", "")
		t.Contains(ver, `"version":"`+t.Want+`"`, "the version API")
		_, page := call("GET", "/", "", "")
		t.ContainsFold(page, "langflow", "the React UI")
		c, b := call("POST", "/api/v1/flows/", tok, `{"name":"quench-smoke","description":"smoke","data":{"nodes":[],"edges":[]}}`)
		var flow struct {
			ID string `json:"id"`
		}
		if c != 201 && c != 200 || json.Unmarshal([]byte(b), &flow) != nil || flow.ID == "" {
			t.Fatalf("create flow: %d %s", c, b)
		}
		docker("stop", "-t", "30", name)
		if logs := t.Logs(name); strings.Contains(logs, "Traceback") {
			t.Fatalf("traceback in the logs:\n%s", logs)
		}
		docker("rm", "-f", name)
		name = t.Name("again")
		tok = start(name)
		c, b = call("GET", "/api/v1/flows/"+flow.ID, tok, "")
		if c != 200 || !strings.Contains(b, "quench-smoke") {
			t.Fatalf("flow after a restart: %d %s", c, b)
		}
		t.Logf("smoke test passed (langflow %s: read-only boot, version API, React UI, auto-login refused, wrong password 401, superuser token, flow saved and read back after a restart on the volume; user %s)", t.Want, u)
	})
}
