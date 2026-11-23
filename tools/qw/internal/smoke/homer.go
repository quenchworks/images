package smoke

import (
	"os"
	"path/filepath"
	"time"
)

func init() {
	// On a read-only rootfs with only /tmp writable: nginx as 1001 serves the app shell and the default
	// assets/config.yml, an unknown path falls back to the SPA, and a config.yml mounted over the default is the
	// one served (the chart mounts its ConfigMap the same way).
	register("homer", func(t *T) {
		const base = "http://127.0.0.1:18093"
		u := t.User("1001")
		dir, err := os.MkdirTemp("", "homer-smoke-")
		if err != nil {
			t.Fatalf("temp dir: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(dir) })
		conf := filepath.Join(dir, "config.yml")
		if err := os.WriteFile(conf, []byte("title: \"Quench Smoke\"\nservices: []\n"), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}
		flags := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-p", "127.0.0.1:18093:8080"}

		name := t.Name("")
		t.Start(name, flags)
		t.waitCode(base+"/", 30, time.Second, name, 200)
		_, page := restDo("GET", base+"/", "", "", "")
		t.ContainsFold(page, "homer", "the app shell")
		_, def := restDo("GET", base+"/assets/config.yml", "", "", "")
		t.Contains(def, "services:", "the default config.yml")
		if c, _ := restDo("GET", base+"/some/client/route", "", "", ""); c != 200 {
			t.Fatalf("SPA fallback got %d, want 200", c)
		}
		docker("rm", "-f", name)

		name = t.Name("mounted")
		t.Start(name, append(flags, "-v", conf+":/usr/share/nginx/html/assets/config.yml:ro"))
		t.waitCode(base+"/", 30, time.Second, name, 200)
		_, got := restDo("GET", base+"/assets/config.yml", "", "", "")
		t.Contains(got, "Quench Smoke", "the mounted config.yml")
		t.Logf("smoke test passed (homer %s: read-only boot, app shell + default config served, SPA fallback, mounted config.yml served; user %s)", t.Want, u)
	})
}
