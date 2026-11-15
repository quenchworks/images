package smoke

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func init() {
	// On a read-only rootfs with config.toml, a repositories volume and runtime_dir /tmp (it must exist): Gitaly unpacks
	// its bundled Git into runtime_dir (so /tmp needs exec), reports the Git version it uses and serves gRPC on 8075.
	register("gitaly", func(t *T) {
		u := t.User("1001")
		t.Contains(t.RunAny("-version"), "version "+t.Want, "gitaly -version")
		dir, err := os.MkdirTemp(envOr("TMPDIR", ""), "gitaly-smoke-")
		if err != nil {
			t.Fatalf("temp dir: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(dir) })
		_ = os.Chmod(dir, 0o755)
		cfg := `listen_addr = "0.0.0.0:8075"
runtime_dir = "/tmp"
bin_dir = "/usr/bin"
[auth]
token = "smoke-token-0123456789"
# no GitLab behind it in the smoke test; Gitaly only builds the client at start
[gitlab]
url = "http://127.0.0.1:8080"
secret = "smoke-gitlab-secret-0123456789"
[[storage]]
name = "default"
path = "/home/git/repositories"
[logging]
format = "json"
level = "info"
`
		if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(cfg), 0o644); err != nil {
			t.Fatalf("%v", err)
		}
		vol := t.Name("repos")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,exec,mode=1777", "-v", dir + ":/etc/gitaly:ro", "-v", vol + ":/home/git/repositories",
			"-p", "127.0.0.1:18075:8075"})
		t.waitFor(60, time.Second, name, "gitaly never listened on 8075", func() bool {
			if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
				t.Fatalf("gitaly exited\n%s", t.Logs(name))
			}
			c, err := net.DialTimeout("tcp", "127.0.0.1:18075", time.Second)
			if err != nil {
				return false
			}
			c.Close()
			return strings.Contains(t.Logs(name), "using Git version")
		})
		t.Logf("smoke test passed (gitaly %s: read-only boot, bundled Git unpacked and detected, gRPC listening; user %s)", t.Want, u)
	})
}
