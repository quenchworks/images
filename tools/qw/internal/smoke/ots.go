package smoke

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func init() {
	// On a read-only root: the embedded frontend is served, a secret stored through the API reads back once, and the
	// second read of the same link fails (one-time).
	register("ots", func(t *T) {
		const base = "http://127.0.0.1:18131"
		u := t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-p", "127.0.0.1:18131:3000"})
		t.waitFor(30, time.Second, name, "the frontend did not answer", func() bool {
			c, _ := Get(base + "/")
			return c == http.StatusOK
		})
		if c, _ := Get(base + "/api.html"); c != http.StatusOK {
			t.Fatalf("API docs not served (%d)", c)
		}
		j := map[string]string{"Content-Type": "application/json"}
		c, b := DoH("POST", base+"/api/create", `{"secret":"quench-smoke-ciphertext"}`, j)
		var cr struct {
			Success  bool   `json:"success"`
			SecretID string `json:"secret_id"`
		}
		if c >= 300 || json.Unmarshal([]byte(b), &cr) != nil || !cr.Success || cr.SecretID == "" {
			t.Fatalf("create: %d %s", c, b)
		}
		if c, b := Get(base + "/api/get/" + cr.SecretID); c != http.StatusOK || !strings.Contains(b, "quench-smoke-ciphertext") {
			t.Fatalf("first read: %d %s", c, b)
		}
		if c, b := Get(base + "/api/get/" + cr.SecretID); c == http.StatusOK && strings.Contains(b, "quench-smoke-ciphertext") {
			t.Fatalf("the secret read twice: %s", b)
		}
		t.Logf("smoke test passed (ots %s: read-only root, frontend and API docs served, a secret stored and read once, the second read refused; user %s)", t.Want, u)
	})
}
