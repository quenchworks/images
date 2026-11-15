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
	// On a read-only rootfs with YARR_AUTH and SQLite on a volume: the API refuses a request
	// without a session, a wrong password is refused, a login creates a folder through the API,
	// and the folder is still listed after a restart on the same volume.
	register("yarr", func(t *T) {
		const base = "http://127.0.0.1:17070"
		const pw = "Smoke-Pass-2026"
		u := t.User("1001")
		t.Contains(t.RunAny("-version"), t.Want, "yarr -version")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		// an empty named volume is seeded from the image's /data, which apko makes 1001-owned
		start := func(name string) {
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/data", "-p", "127.0.0.1:17070:7070",
				"-e", "YARR_AUTH=smoke:" + pw})
			t.waitFor(60, time.Second, name, "yarr never answered", func() bool {
				if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
					t.Fatalf("yarr exited\n%s", t.Logs(name))
				}
				c, _ := Get(base + "/api/status")
				return c == 401
			})
		}
		jar, _ := cookiejar.New(nil)
		cl := &http.Client{Jar: jar, Timeout: 10 * time.Second}
		login := func(p string) int {
			resp, err := cl.PostForm(base+"/login", url.Values{"username": {"smoke"}, "password": {p}})
			if err != nil {
				t.Fatalf("login: %v", err)
			}
			resp.Body.Close()
			return resp.StatusCode
		}
		folders := func() string {
			resp, err := cl.Get(base + "/api/folders")
			if err != nil || resp.StatusCode != 200 {
				t.Fatalf("folder list: %v %v", err, resp)
			}
			defer resp.Body.Close()
			b, _ := io.ReadAll(resp.Body)
			return string(b)
		}
		name := t.Name("")
		start(name)
		if c := login("wrong-password-1"); c != 401 {
			t.Fatalf("a wrong password answered %d, expected 401", c)
		}
		if c := login(pw); c != 200 {
			t.Fatalf("login answered %d\n%s", c, t.Logs(name))
		}
		resp, err := cl.Post(base+"/api/folders", "application/json", strings.NewReader(`{"title":"quench-smoke"}`))
		if err != nil || resp.StatusCode != 201 {
			t.Fatalf("folder create: %v %v", err, resp)
		}
		resp.Body.Close()
		t.Contains(folders(), "quench-smoke", "folder list after create")
		docker("rm", "-f", name)
		start(t.Name("again"))
		// the session cookie is derived from the credentials, so it is still valid after a restart
		t.Contains(folders(), "quench-smoke", "folder list after a restart on the same volume")
		t.Logf("smoke test passed (yarr %s: read-only boot, 401 without a session, wrong password refused, login, folder kept across a restart; user %s)", t.Want, u)
	})
}
