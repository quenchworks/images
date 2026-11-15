package smoke

import (
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"time"
)

func init() {
	// On a read-only rootfs with a shared secret and no Rails behind it: gitlab-workhorse starts,
	// listens, and answers a proxied request with 502 (it reached its proxy path and failed closed).
	register("gitlab-workhorse", func(t *T) {
		u := t.User("1001")
		t.Contains(t.RunAny("-version"), "v"+t.Want, "gitlab-workhorse -version")
		dir, err := os.MkdirTemp(envOr("TMPDIR", ""), "workhorse-smoke-")
		if err != nil {
			t.Fatalf("temp dir: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(dir) })
		_ = os.Chmod(dir, 0o755)
		key := make([]byte, 32)
		_, _ = rand.Read(key)
		if err := os.WriteFile(filepath.Join(dir, "secret"), []byte(base64.StdEncoding.EncodeToString(key)), 0o644); err != nil {
			t.Fatalf("%v", err)
		}
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", dir + ":/etc/gitlab-workhorse:ro", "-p", "127.0.0.1:18181:8181"},
			"-listenAddr", "0.0.0.0:8181", "-authBackend", "http://127.0.0.1:1", "-secretPath", "/etc/gitlab-workhorse/secret", "-documentRoot", "/tmp")
		var code int
		t.waitFor(30, time.Second, name, "gitlab-workhorse never answered", func() bool {
			if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
				t.Fatalf("gitlab-workhorse exited\n%s", t.Logs(name))
			}
			code, _ = Get("http://127.0.0.1:18181/api/v4/version")
			return code != 0
		})
		if code != 502 {
			t.Fatalf("a request with no Rails behind it answered %d, expected 502\n%s", code, t.Logs(name))
		}
		t.Logf("smoke test passed (gitlab-workhorse %s: read-only boot, listening, proxy fails closed with 502 when Rails is down; user %s)", t.Want, u)
	})
}
