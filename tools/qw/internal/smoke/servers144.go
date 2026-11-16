package smoke

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// chhotoDo sends a request (no redirects followed) and returns the status, body and Location.
func chhotoDo(method, url, ctype, body string, hdr map[string]string) (int, string, string) {
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	r, err := noRedirect.Do(req)
	if err != nil {
		return 0, err.Error(), ""
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return r.StatusCode, string(b), r.Header.Get("Location")
}

func init() {
	// On a read-only rootfs with /var/lib/chhoto-url on a volume: the UI and /api/version answer, a wrong API key
	// cannot add a link, the right one adds a link that redirects to its target, a wrong password is refused and the
	// right one logs in, and the link survives a restart.
	register("chhoto-url", func(t *T) {
		const base = "http://127.0.0.1:18096"
		const pw = "Smoke-Chhoto-2026!"
		const target = "https://quench-works.com/smoke"
		u := t.User("1001")
		raw := make([]byte, 64)
		rand.Read(raw)
		key := base64.RawURLEncoding.EncodeToString(raw)
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		start := func(name string) {
			t.Start(name, []string{"--read-only", "-v", vol + ":/var/lib/chhoto-url", "-p", "127.0.0.1:18096:4567",
				"-e", "CHHOTO_PASSWORD=" + pw, "-e", "CHHOTO_API_KEY=" + key})
			t.WaitHTTP(base+"/api/version", 60, name)
		}
		name := t.Name("")
		start(name)
		_, ver := Get(base + "/api/version")
		if t.Want != "" {
			t.Contains(ver, "v"+t.Want, "/api/version")
		}
		_, page := Get(base + "/")
		t.Contains(page, "Chhoto", "the web UI")
		link := `{"shortlink":"quench-smoke","longlink":"` + target + `"}`
		if _, b, _ := chhotoDo("POST", base+"/api/new", "application/json", link, map[string]string{"X-API-Key": "wrong-key-1"}); !strings.Contains(b, `"success":false`) {
			t.Fatalf("a wrong API key added a link: %s", b)
		}
		_, b, _ := chhotoDo("POST", base+"/api/new", "application/json", link, map[string]string{"X-API-Key": key})
		var res struct {
			Success  bool
			Shorturl string
		}
		if json.Unmarshal([]byte(b), &res) != nil || !res.Success {
			t.Fatalf("adding a link with the API key: %s\n%s", b, t.Logs(name))
		}
		redirect := func() {
			if c, _, loc := chhotoDo("GET", base+"/quench-smoke", "", "", nil); c/100 != 3 || loc != target {
				t.Fatalf("/quench-smoke: %d -> %q, want a redirect to %s", c, loc, target)
			}
		}
		redirect()
		if c, _, _ := chhotoDo("POST", base+"/api/login", "text/plain", "wrong-password-1", nil); c != http.StatusUnauthorized {
			t.Fatalf("a wrong password got %d, want 401", c)
		}
		if c, b, _ := chhotoDo("POST", base+"/api/login", "text/plain", pw, nil); c != http.StatusOK {
			t.Fatalf("login: %d %s", c, b)
		}
		docker("rm", "-f", name)
		time.Sleep(time.Second)
		start(t.Name("again"))
		redirect()
		t.Logf("smoke test passed (chhoto-url %s: read-only boot, UI and version, wrong API key refused, link added and redirecting, wrong password refused, login, link kept across a restart; user %s)", t.Want, u)
	})
}
