package smoke

import (
	"strings"
	"time"
)

func init() {
	register("it-tools", itTools)
	register("it-tools-sharevb", itTools)
}

// itTools: on a read-only rootfs with only /tmp writable, nginx as 1001 serves the app shell, its entry bundle and
// a deep link to a tool through the SPA fallback. Both IT-Tools images (upstream and the sharevb fork) build the
// same way.
func itTools(t *T) {
	const base = "http://127.0.0.1:18099"
	u := t.User("1001")
	name := t.Name("")
	t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-p", "127.0.0.1:18099:8080"})
	t.waitCode(base+"/", 30, time.Second, name, 200)
	_, page := restDo("GET", base+"/", "", "", "")
	t.ContainsFold(page, "it-tools", "the app shell")
	js := strings.TrimPrefix(t.Match(page, `src="(\.?/?assets/[^"]+\.js)"`, "the entry bundle"), ".")
	if !strings.HasPrefix(js, "/") {
		js = "/" + js
	}
	if c, _ := restDo("GET", base+js, "", "", ""); c != 200 {
		t.Fatalf("entry bundle %s got %d", js, c)
	}
	if c, b := restDo("GET", base+"/uuid-generator", "", "", ""); c != 200 || !strings.Contains(strings.ToLower(b), "it-tools") {
		t.Fatalf("deep link /uuid-generator got %d", c)
	}
	t.Logf("smoke test passed (%s: read-only boot, app shell, entry bundle, SPA deep link; user %s)", t.Want, u)
}
