package smoke

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func init() {
	// On a read-only rootfs with /var/lib/etherpad on a volume (dirty DB, API key, session key): the server answers
	// /health, a pad is created and read back through the HTTP API with the generated key, a wrong key is refused, and
	// after a restart the same key still works and the pad text is still there.
	register("etherpad", func(t *T) {
		const base = "http://127.0.0.1:18116"
		const text = "Quench smoke pad"
		u := t.User("1001")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		flags := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/var/lib/etherpad", "-p", "127.0.0.1:18116:9001",
			"-e", "AUTHENTICATION_METHOD=apikey", "-e", "ADMIN_PASSWORD=Smoke-Etherpad-2026"}
		start := func(name string) {
			t.Start(name, flags)
			t.waitFor(90, 2*time.Second, name, "etherpad did not answer /health", func() bool {
				c, _ := restDo("GET", base+"/health", "", "", "")
				return c == http.StatusOK
			})
		}
		apikey := func(name string) string {
			dir, _ := os.MkdirTemp("", "etherpad-smoke")
			defer os.RemoveAll(dir)
			t.Docker("cp", name+":/var/lib/etherpad/APIKEY.txt", dir)
			b, err := os.ReadFile(filepath.Join(dir, "APIKEY.txt"))
			if err != nil || len(strings.TrimSpace(string(b))) < 16 {
				t.Fatalf("no APIKEY.txt on the volume: %v", err)
			}
			return strings.TrimSpace(string(b))
		}
		call := func(fn, key string, q url.Values) (int, string) {
			q.Set("apikey", key)
			return restDo("GET", base+"/api/1.3.1/"+fn+"?"+q.Encode(), "", "", "")
		}
		name := t.Name("")
		start(name)
		key := apikey(name)
		if c, b := call("createPad", key, url.Values{"padID": {"quench-smoke"}, "text": {text}}); c != http.StatusOK || !strings.Contains(b, `"code":0`) {
			t.Fatalf("createPad: %d %s\n%s", c, b, t.Logs(name))
		}
		if c, _ := call("getText", "wrong-key-0000000000000000", url.Values{"padID": {"quench-smoke"}}); c == http.StatusOK {
			t.Fatalf("a wrong API key was accepted")
		}
		read := func() {
			c, b := call("getText", key, url.Values{"padID": {"quench-smoke"}})
			var r struct {
				Data struct{ Text string } `json:"data"`
			}
			if c != http.StatusOK || json.Unmarshal([]byte(b), &r) != nil || !strings.Contains(r.Data.Text, text) {
				t.Fatalf("getText: %d %s", c, b)
			}
		}
		read()
		docker("rm", "-f", name)
		start(t.Name("again"))
		if k := apikey(t.Name("again")); k != key {
			t.Fatalf("the API key changed across a restart")
		}
		read()
		t.Logf("smoke test passed (etherpad %s: read-only boot, /health, pad created and read back over the HTTP API, wrong key refused, key and pad kept across a restart; user %s)", t.Want, u)
	})
}
