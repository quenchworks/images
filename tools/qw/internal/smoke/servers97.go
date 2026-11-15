package smoke

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"
)

// privatebinPaste is a well-formed v2 paste. The server never decrypts: it checks the shape
// (cipher parameters, base64 fields, sizes) and that the ciphertext looks random, so random
// bytes stand in for a browser-encrypted paste.
func privatebinPaste() string {
	b := func(n int) string {
		r := make([]byte, n)
		_, _ = rand.Read(r)
		return base64.StdEncoding.EncodeToString(r)
	}
	p := map[string]any{
		"v":     2,
		"adata": []any{[]any{b(16), b(8), 100000, 256, 128, "aes", "gcm", "zlib"}, "plaintext", 0, 0},
		"ct":    b(96),
		"meta":  map[string]any{"expire": "1week"},
	}
	j, _ := json.Marshal(p)
	return string(j)
}

func init() {
	// php-fpm behind nginx under supervisor on a read-only rootfs, pastes on a volume: the
	// front page, a paste created through the JSON API and read back by id, a malformed paste
	// refused, and the paste still there after a restart on the same volume.
	register("privatebin", func(t *T) {
		const base = "http://127.0.0.1:18090"
		u := t.User("1001")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		// an empty named volume is seeded from the image's /srv/data, which apko makes 1001-owned
		start := func(name string) {
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/srv/data", "-p", "127.0.0.1:18090:8080"})
			t.waitFor(60, time.Second, name, "privatebin never served its front page", func() bool {
				if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
					t.Fatalf("privatebin exited\n%s", t.Logs(name))
				}
				c, b := Get(base + "/")
				return c == 200 && strings.Contains(b, "PrivateBin")
			})
		}
		api := map[string]string{"X-Requested-With": "JSONHttpRequest", "Content-Type": "application/json"}
		name := t.Name("")
		start(name)
		_, b := Get(base + "/")
		t.Contains(b, t.Want, "version on the front page")

		_, out := DoH("POST", base+"/", privatebinPaste(), api)
		var res struct {
			Status int    `json:"status"`
			ID     string `json:"id"`
		}
		if err := json.Unmarshal([]byte(out), &res); err != nil || res.Status != 0 || res.ID == "" {
			t.Fatalf("paste create: %s (%v)\n%s", out, err, t.Logs(name))
		}
		read := func() string {
			_, r := DoH("GET", base+"/?pasteid="+res.ID, "", api)
			return r
		}
		t.Contains(read(), `"status":0`, "paste read back by id")
		// the traffic limiter allows one post per 10 s per client; then a malformed paste must be
		// refused for its shape, not for the rate
		time.Sleep(11 * time.Second)
		if _, out := DoH("POST", base+"/", `{"v":2,"adata":[],"ct":"x","meta":{"expire":"1week"}}`, api); !strings.Contains(out, "Invalid data") {
			t.Fatalf("a malformed paste was not refused as invalid: %s", out)
		}
		docker("rm", "-f", name)
		start(t.Name("again"))
		t.Contains(read(), `"status":0`, "paste read back after a restart on the same volume")
		t.Logf("smoke test passed (privatebin %s: php-fpm + nginx on a read-only rootfs, paste created and read back, malformed paste refused, kept across a restart; user %s)", t.Want, u)
	})
}
