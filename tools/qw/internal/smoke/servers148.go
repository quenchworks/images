package smoke

import (
	"io"
	"net/http"
	"strings"
	"time"
)

// sbDo sends a request (no redirects followed) with an optional bearer token and returns the status and body.
func sbDo(method, url, token, body string) (int, string) {
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	r, err := noRedirect.Do(req)
	if err != nil {
		return 0, err.Error()
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return r.StatusCode, string(b)
}

func init() {
	// On a read-only rootfs with /data on a volume and SB_USER/SB_AUTH_TOKEN set: /.ping answers, an anonymous or
	// wrong-token write is refused, the token writes a page and reads it back, shell commands are refused, and the
	// page survives a restart.
	register("silverbullet", func(t *T) {
		if !strings.HasPrefix(t.Want, "2.") {
			sb3(t)
			return
		}
		const base = "http://127.0.0.1:18101"
		const token = "quench-smoke-token-0123456789abcdef"
		const page = "# Quench smoke\n\nwritten by the smoke test\n"
		u := t.User("1001")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		start := func(name string) {
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/data", "-p", "127.0.0.1:18101:3000",
				"-e", "SB_USER=smoke:Smoke-Bullet-2026!", "-e", "SB_AUTH_TOKEN=" + token})
			t.WaitHTTP(base+"/.ping", 60, name)
		}
		read := func() {
			if c, b := sbDo("GET", base+"/.fs/quench-smoke.md", token, ""); c != http.StatusOK || b != page {
				t.Fatalf("read the page: %d %q", c, b)
			}
		}
		name := t.Name("")
		start(name)
		if c, _ := sbDo("PUT", base+"/.fs/intruder.md", "", "x"); c/100 == 2 {
			t.Fatalf("an anonymous write answered %d", c)
		}
		if c, _ := sbDo("PUT", base+"/.fs/intruder.md", "wrong-token-1", "x"); c/100 == 2 {
			t.Fatalf("a wrong token wrote a page (%d)", c)
		}
		if c, b := sbDo("PUT", base+"/.fs/quench-smoke.md", token, page); c/100 != 2 {
			t.Fatalf("write the page: %d %s\n%s", c, b, t.Logs(name))
		}
		read()
		if c, b := sbDo("POST", base+"/.shell", token, `{"cmd":"id","args":[]}`); c/100 == 2 && !strings.Contains(strings.ToLower(b), "disabled") {
			t.Fatalf("a shell command ran (%d %s)", c, b)
		}
		docker("rm", "-f", name)
		time.Sleep(time.Second)
		start(t.Name("again"))
		read()
		t.Logf("smoke test passed (silverbullet %s: read-only boot, anonymous and wrong-token writes refused, page written and read with the token, shell refused, page kept across a restart; user %s)", t.Want, u)
	})
}
