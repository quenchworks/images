package smoke

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

func init() {
	// On a read-only rootfs with /config on a volume: upstream's startup runs nginx, gunicorn and the pruner as uid
	// 1001, /status answers, a config stored under a key delivers a notification to a JSON webhook on the host, an
	// unknown key is refused, and the stored config still delivers after a restart.
	register("apprise-api", func(t *T) {
		const base = "http://127.0.0.1:18099"
		u := t.User("1001")
		var mu sync.Mutex
		var got []string
		hook := t.hostStub(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			mu.Lock()
			got = append(got, string(b))
			mu.Unlock()
		}))
		vol := t.Name("config")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		start := func(name string) {
			t.Start(name, append([]string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--tmpfs", "/attach:rw,uid=1001,gid=1001",
				"-v", vol + ":/config", "-p", "127.0.0.1:18099:8000"}, hostStubFlag...))
			t.WaitHTTP(base+"/status", 90, name)
		}
		form := func(path string, v url.Values) (int, string) {
			return DoH("POST", base+path, v.Encode(), map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
		}
		delivered := func(msg string) bool {
			for i := 0; i < 20; i++ {
				mu.Lock()
				for _, g := range got {
					if strings.Contains(g, msg) {
						mu.Unlock()
						return true
					}
				}
				mu.Unlock()
				time.Sleep(500 * time.Millisecond)
			}
			return false
		}
		name := t.Name("")
		start(name)
		target := "json://" + strings.TrimPrefix(hook, "http://") + "/hook"
		if c, b := form("/add/quench-smoke", url.Values{"urls": {target}}); c != http.StatusOK {
			t.Fatalf("store the config: %d %s\n%s", c, b, t.Logs(name))
		}
		if c, b := form("/notify/quench-smoke", url.Values{"title": {"Quench"}, "body": {"quench smoke one"}}); c != http.StatusOK || !delivered("quench smoke one") {
			t.Fatalf("notify: %d %s, delivered %v\n%s", c, b, delivered("quench smoke one"), t.Logs(name))
		}
		if c, _ := form("/notify/no-such-key", url.Values{"body": {"x"}}); c < 400 {
			t.Fatalf("an unknown key answered %d", c)
		}
		docker("rm", "-f", name)
		start(t.Name("again"))
		if c, b := form("/notify/quench-smoke", url.Values{"title": {"Quench"}, "body": {"quench smoke two"}}); c != http.StatusOK || !delivered("quench smoke two") {
			t.Fatalf("notify after a restart: %d %s", c, b)
		}
		t.Logf("smoke test passed (apprise-api %s: read-only boot of upstream's runtime, config stored under a key, notification delivered to a JSON webhook, unknown key refused, config kept across a restart; user %s)", t.Want, u)
	})
}
