package smoke

import (
	"net/http"
	"strings"
)

func init() {
	// On a read-only rootfs (only /tmp writable): the app shell and its assets are served, config.js keeps ChartDB
	// Cloud links hidden and analytics off with no OpenAI key, and SPA routes fall back to index.html.
	register("chartdb", func(t *T) {
		const base = "http://127.0.0.1:18103"
		u := t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-p", "127.0.0.1:18103:8080"})
		page := t.WaitHTTP(base+"/", 30, name)
		t.Contains(page, "<title>ChartDB", "the app shell")
		t.Contains(page, `src="/config.js"`, "the config.js reference")
		_, cfg := Get(base + "/config.js")
		t.Contains(cfg, `HIDE_CHARTDB_CLOUD: "true"`, "config.js")
		t.Contains(cfg, `DISABLE_ANALYTICS: "true"`, "config.js")
		t.Contains(cfg, `OPENAI_API_KEY: ""`, "config.js")
		if c, b := Get(base + "/diagrams/quench-smoke"); c != http.StatusOK || !strings.Contains(b, "<title>ChartDB") {
			t.Fatalf("SPA fallback: %d", c)
		}
		i := strings.Index(page, `src="/assets/`)
		if i < 0 {
			t.Fatalf("no bundled script in the app shell")
		}
		asset := page[i+5 : i+5+strings.Index(page[i+5:], `"`)]
		if c, _ := Get(base + asset); c != http.StatusOK {
			t.Fatalf("GET %s: %d", asset, c)
		}
		t.Logf("smoke test passed (chartdb %s: read-only boot, app shell and bundle served, config.js with cloud links hidden and analytics off, SPA fallback; user %s)", t.Want, u)
	})
}
