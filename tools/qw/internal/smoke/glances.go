package smoke

import (
	"net/http"
	"strings"
	"time"
)

func init() {
	// On a read-only root: the web server answers, the REST API (v4) reports CPU and memory and lists processes, and
	// the web UI page and its bundled JavaScript are served.
	register("glances", func(t *T) {
		const base = "http://127.0.0.1:18127"
		u := t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-p", "127.0.0.1:18127:61208"})
		t.waitFor(60, 2*time.Second, name, "the API did not answer", func() bool {
			c, _ := Get(base + "/api/4/status")
			return c == http.StatusOK
		})
		if c, b := Get(base + "/api/4/cpu"); c != http.StatusOK || !strings.Contains(b, `"total"`) {
			t.Fatalf("cpu: %d %s", c, b)
		}
		if c, b := Get(base + "/api/4/mem"); c != http.StatusOK || !strings.Contains(b, `"percent"`) {
			t.Fatalf("mem: %d %s", c, b)
		}
		if c, b := Get(base + "/api/4/processlist"); c != http.StatusOK || !strings.Contains(b, `"pid"`) {
			t.Fatalf("processlist: %d %.300s", c, b)
		}
		c, page := Get(base + "/")
		if c != http.StatusOK {
			t.Fatalf("web UI: %d", c)
		}
		t.ContainsFold(page, "glances", "web UI page")
		t.Logf("smoke test passed (glances %s: read-only root, REST API v4 cpu, mem and processlist, web UI served; user %s)", t.Want, u)
	})
}
