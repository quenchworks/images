package smoke

import (
	"io"
	"net/http"
	"strings"
	"time"
)

func init() {
	// On a read-only root: the embedded web UI is served, the download endpoint streams the requested amount, the
	// upload endpoint takes a body, getIP answers, and the stats page stays disabled with the default password.
	register("librespeed", func(t *T) {
		const base = "http://127.0.0.1:18128"
		u := t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-p", "127.0.0.1:18128:8989"})
		t.waitFor(30, time.Second, name, "the web UI did not answer", func() bool {
			c, _ := Get(base + "/")
			return c == http.StatusOK
		})
		_, page := Get(base + "/")
		t.ContainsFold(page, "librespeed", "web UI")
		r, err := http.Get(base + "/garbage?ckSize=2")
		if err != nil {
			t.Fatalf("garbage: %v", err)
		}
		n, _ := io.Copy(io.Discard, r.Body)
		r.Body.Close()
		if r.StatusCode != http.StatusOK || n < 2*1024*1024 {
			t.Fatalf("garbage: %d, %d bytes (want 2 MiB)", r.StatusCode, n)
		}
		if c, b := DoH("POST", base+"/empty", strings.Repeat("x", 256*1024), map[string]string{"Content-Type": "application/octet-stream"}); c != http.StatusOK {
			t.Fatalf("empty (upload): %d %s", c, b)
		}
		if c, b := Get(base + "/getIP"); c != http.StatusOK || !strings.Contains(b, "processedString") {
			t.Fatalf("getIP: %d %s", c, b)
		}
		if _, b := Get(base + "/stats"); !strings.Contains(b, "Please set statistics_password") {
			t.Fatalf("the stats page is not disabled by default: %.200s", b)
		}
		t.Logf("smoke test passed (librespeed %s: read-only root, web UI, 2 MiB download, upload, getIP, stats disabled by default; user %s)", t.Want, u)
	})
}
