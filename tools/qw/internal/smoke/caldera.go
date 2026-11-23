package smoke

import (
	"net/http"
	"regexp"
	"strings"
	"time"
)

func init() {
	// First start without --insecure: Caldera writes conf/local.yml with random credentials and keys (none of
	// the default.yml ones work), serves the magma UI, answers the API with the generated red key and refuses a
	// wrong one, lists the stock abilities, and serves the sandcat and manx agents built from source. A graceful
	// stop saves its configs without a permission error.
	register("caldera", func(t *T) {
		const base = "http://127.0.0.1:18894"
		u := t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"-p", "127.0.0.1:18894:8888"})
		t.waitCode(base+"/", 90, 2*time.Second, name, 200)
		_, page := restDo("GET", base+"/", "", "", "")
		t.Contains(page, "Caldera", "the magma UI")

		conf, err := docker("cp", name+":/opt/caldera/conf/local.yml", "-")
		if err != nil {
			t.Fatalf("no generated conf/local.yml: %v\n%s", err, t.Logs(name))
		}
		key := t.Match(conf, `api_key_red: (\S+)`, "the generated red API key")
		if key == "ADMIN123" || strings.Contains(conf, "admin: admin") || strings.Contains(conf, "app.contact.ftp.pword: caldera\n") {
			t.Fatalf("local.yml kept a default credential:\n%s", conf)
		}
		api := func(k, path string) (int, string) {
			req, _ := http.NewRequest("GET", base+path, nil)
			req.Header.Set("KEY", k)
			return insecureDo(req)
		}
		if c, _ := api("ADMIN123", "/api/v2/abilities"); c != http.StatusUnauthorized {
			t.Fatalf("the default.yml red key got %d, want 401", c)
		}
		c, body := api(key, "/api/v2/abilities")
		if c != http.StatusOK {
			t.Fatalf("abilities with the generated key: %d\n%s", c, t.Logs(name))
		}
		n := len(regexp.MustCompile(`"ability_id"`).FindAllString(body, -1))
		if n < 100 {
			t.Fatalf("only %d abilities loaded\n%s", n, t.Logs(name))
		}
		_, health := api(key, "/api/v2/health")
		t.Contains(health, `"version": "`+t.Want, "the health version")
		for _, agent := range []string{"sandcat.go", "manx.go"} {
			req, _ := http.NewRequest("POST", base+"/file/download", nil)
			req.Header.Set("file", agent)
			req.Header.Set("platform", "linux")
			c, b := insecureDo(req)
			if c != http.StatusOK || len(b) < 1<<20 || !strings.HasPrefix(b, "\x7fELF") {
				t.Fatalf("%s download: %d, %d bytes", agent, c, len(b))
			}
		}
		docker("stop", "-t", "30", name)
		if logs := t.Logs(name); strings.Contains(logs, "Permission denied") || strings.Contains(logs, "Traceback") || strings.Contains(logs, "Error enabling plugin") {
			t.Fatalf("errors on run or shutdown:\n%s", logs)
		}
		t.Logf("smoke test passed (caldera %s: random local.yml on first start, default key refused, generated key lists %d abilities, sandcat + manx served, clean shutdown; user %s)", t.Want, n, u)
	})
}
