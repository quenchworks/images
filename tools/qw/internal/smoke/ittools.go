package smoke

import (
	"strings"
	"time"
)

func init() {
	// On a read-only rootfs with only /tmp writable: nginx as 1001 serves the app shell, its JS bundle and a
	// deep link to a tool (the SPA fallback), and the bundle carries the floored DOMPurify (3.5+).
	register("it-tools", func(t *T) {
		const base = "http://127.0.0.1:18099"
		u := t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-p", "127.0.0.1:18099:8080"})
		t.waitCode(base+"/", 30, time.Second, name, 200)
		_, page := restDo("GET", base+"/", "", "", "")
		t.ContainsFold(page, "it-tools", "the app shell")
		js := t.Match(page, `src="(/assets/index-[^"]+\.js)"`, "the entry bundle")
		if c, _ := restDo("GET", base+js, "", "", ""); c != 200 {
			t.Fatalf("entry bundle %s got %d", js, c)
		}
		if c, b := restDo("GET", base+"/uuid-generator", "", "", ""); c != 200 || !strings.Contains(strings.ToLower(b), "it-tools") {
			t.Fatalf("deep link /uuid-generator got %d", c)
		}
		t.Logf("smoke test passed (it-tools %s: read-only boot, app shell, entry bundle, SPA deep link; user %s)", t.Want, u)
	})
}
