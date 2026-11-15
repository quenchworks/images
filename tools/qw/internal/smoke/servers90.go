package smoke

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func init() {
	// On a read-only rootfs: /api/health, a signup and login through the forms, the API key read
	// from the settings page, a heartbeat sent through the WakaTime-compatible API, and its
	// project back in today's summary; then the same after a restart on the same volume.
	register("wakapi", func(t *T) {
		const base = "http://127.0.0.1:13000"
		const pw = "Smoke-Pass-2026"
		u := t.User("1001")
		t.Contains(t.RunAny("-version"), t.Want, "wakapi -version")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		// an empty named volume is seeded from the image's /data, which apko makes 1001-owned
		start := func(name string) {
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/data", "-p", "127.0.0.1:13000:3000",
				"-e", "WAKAPI_PASSWORD_SALT=smoke-salt-0123456789"})
			t.waitFor(60, time.Second, name, "wakapi never answered /api/health", func() bool {
				if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
					t.Fatalf("wakapi exited\n%s", t.Logs(name))
				}
				c, _ := Get(base + "/api/health")
				return c == 200
			})
		}
		name := t.Name("")
		start(name)
		jar, _ := cookiejar.New(nil)
		cl := &http.Client{Jar: jar, Timeout: 15 * time.Second}
		form := func(path string, v url.Values) {
			resp, err := cl.PostForm(base+path, v)
			if err != nil || resp.StatusCode >= 400 {
				t.Fatalf("POST %s: %v %v", path, err, resp)
			}
			resp.Body.Close()
		}
		form("/signup", url.Values{"username": {"smoke"}, "email": {"smoke@example.com"}, "password": {pw}, "password_repeat": {pw}})
		form("/login", url.Values{"username": {"smoke"}, "password": {pw}})
		resp, err := cl.Get(base + "/settings")
		if err != nil {
			t.Fatalf("settings: %v", err)
		}
		page, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		key := t.Match(string(page), `([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})`, "API key on the settings page")
		auth := map[string]string{"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte(key)), "Content-Type": "application/json"}
		hb := `{"entity":"/src/main.go","type":"file","category":"coding","project":"quench-smoke","language":"Go","is_write":true,"time":` +
			strconv.FormatInt(time.Now().Unix(), 10) + `}`
		if c, b := DoH("POST", base+"/api/heartbeat", hb, auth); c >= 300 {
			t.Fatalf("heartbeat: %d %s", c, b)
		}
		summary := func() string {
			_, b := DoH("GET", base+"/api/compat/wakatime/v1/users/current/summaries?range=today", "", auth)
			return b
		}
		// summaries are computed asynchronously from heartbeats
		t.waitFor(20, time.Second, name, "the heartbeat never reached today's summary", func() bool { return strings.Contains(summary(), "quench-smoke") })
		docker("rm", "-f", name)
		start(t.Name("again"))
		t.Contains(summary(), "quench-smoke", "summary after a restart on the same volume")
		t.Logf("smoke test passed (wakapi %s: read-only boot, signup, API key, heartbeat in today's summary, kept across a restart; user %s)", t.Want, u)
	})
}
