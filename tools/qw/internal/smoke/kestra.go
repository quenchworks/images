package smoke

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

const kestraConf = `kestra:
  server:
    basic-auth:
      username: admin@example.com
      password: Smoke-Kestra-2026
  plugins:
    auto-install:
      enabled: false
  tutorial-flows:
    enabled: false
  storage:
    type: local
    local:
      base-path: /app/storage
  tasks:
    tmp-dir:
      path: /tmp/kestra-wd
  url: http://localhost:8080/
`

func init() {
	// server local (embedded H2, local storage on the /app volume) on a read-only root: the API answers behind basic
	// auth, a wrong password is refused, a flow created through the API runs to SUCCESS.
	register("kestra", func(t *T) {
		const base = "http://127.0.0.1:18126"
		u := t.User("1001")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", "-f", vol) })
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/app", "-p", "127.0.0.1:18126:8080",
			"-e", "KESTRA_CONFIGURATION=" + kestraConf})
		auth := func(pw string) string {
			return "Basic " + base64.StdEncoding.EncodeToString([]byte("admin@example.com:"+pw))
		}
		ok := map[string]string{"Authorization": auth("Smoke-Kestra-2026"), "Accept": "application/json"}
		t.waitFor(150, 2*time.Second, name, "the flows API did not answer", func() bool {
			c, _ := DoH("GET", base+"/api/v1/main/flows/search", "", ok)
			return c == http.StatusOK
		})
		if c, _ := DoH("GET", base+"/api/v1/main/flows/search", "", map[string]string{"Authorization": auth("wrong-password-1")}); c == http.StatusOK {
			t.Fatalf("a wrong password was accepted")
		}
		flow := "id: smoke\nnamespace: quench.smoke\ntasks:\n  - id: hello\n    type: io.kestra.plugin.core.log.Log\n    message: hello from the quench smoke test\n"
		h := map[string]string{"Authorization": auth("Smoke-Kestra-2026"), "Content-Type": "application/x-yaml", "Accept": "application/json"}
		if c, b := DoH("POST", base+"/api/v1/main/flows", flow, h); c != http.StatusOK {
			t.Fatalf("create flow: %d %s\n%s", c, b, t.Logs(name))
		}
		c, b := DoH("POST", base+"/api/v1/main/executions/quench.smoke/smoke", "", map[string]string{"Authorization": auth("Smoke-Kestra-2026"), "Accept": "application/json"})
		var ex struct {
			ID string `json:"id"`
		}
		if c != http.StatusOK || json.Unmarshal([]byte(b), &ex) != nil || ex.ID == "" {
			t.Fatalf("start execution: %d %s", c, b)
		}
		var state string
		t.waitFor(60, 2*time.Second, name, "the execution did not finish", func() bool {
			_, state = DoH("GET", base+"/api/v1/main/executions/"+ex.ID, "", ok)
			return strings.Contains(state, `"current":"SUCCESS"`) || strings.Contains(state, `"current":"FAILED"`)
		})
		if !strings.Contains(state, `"current":"SUCCESS"`) {
			t.Fatalf("the execution did not succeed: %s", state)
		}
		t.Logf("smoke test passed (kestra %s: server local on a read-only root, basic auth, wrong password refused, a flow created and its execution SUCCESS; user %s)", t.Want, u)
	})
}
