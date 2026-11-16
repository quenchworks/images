package smoke

import (
	"io"
	"net/http"
	"strings"
	"time"
)

// copypartyDo sends a request and returns the status and body.
func copypartyDo(method, url, body string) (int, string) {
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	r, err := noRedirect.Do(req)
	if err != nil {
		return 0, err.Error()
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return r.StatusCode, string(b)
}

func init() {
	// On a read-only rootfs with /srv on a volume: an account uploads a file to a volume only it may use,
	// reads it back, an anonymous read is refused, and the file survives a restart.
	register("copyparty", func(t *T) {
		const base = "http://127.0.0.1:18082"
		const pw = "Smoke-Pass-2026"
		const text = "quench copyparty smoke file"
		u := t.User("1001")
		t.Contains(t.RunAny("--version"), t.Want, "copyparty --version")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		flags := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--tmpfs", "/var/lib/copyparty:rw,uid=1001,gid=1001",
			"-v", vol + ":/srv", "-p", "127.0.0.1:18082:3923"}
		args := []string{"-p", "3923", "-a", "smoke:" + pw, "-v", "/srv:files:rw,smoke"}
		start := func(name string) {
			t.Start(name, flags, args...)
			for i := 0; i < 60; i++ {
				if c, _ := copypartyDo("GET", base+"/?pw="+pw, ""); c == http.StatusOK {
					return
				}
				time.Sleep(time.Second)
			}
			t.Fatalf("copyparty did not answer:\n%s", t.Logs(name))
		}
		name := t.Name("")
		start(name)
		if c, body := copypartyDo("PUT", base+"/files/hello.txt?pw="+pw, text); c >= 300 {
			t.Fatalf("upload: %d %s", c, body)
		}
		if c, body := copypartyDo("GET", base+"/files/hello.txt?pw="+pw, ""); c != http.StatusOK || body != text {
			t.Fatalf("download: %d %q", c, body)
		}
		if c, body := copypartyDo("GET", base+"/files/hello.txt", ""); c == http.StatusOK || strings.Contains(body, text) {
			t.Fatalf("an anonymous read got %d", c)
		}
		if c, _ := copypartyDo("GET", base+"/files/hello.txt?pw=wrong-password-1", ""); c == http.StatusOK {
			t.Fatalf("a wrong password read the file (%d)", c)
		}
		docker("rm", "-f", name)
		start(t.Name("again"))
		if c, body := copypartyDo("GET", base+"/files/hello.txt?pw="+pw, ""); c != http.StatusOK || body != text {
			t.Fatalf("download after a restart: %d %q", c, body)
		}
		t.Logf("smoke test passed (copyparty %s: read-only boot, account uploaded and read back a file, anonymous and wrong-password reads refused, file kept across a restart; user %s)", t.Want, u)
	})
}
