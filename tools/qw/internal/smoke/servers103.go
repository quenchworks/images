package smoke

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

func init() {
	// On a read-only rootfs with a config and a generated host key: gitlab-sshd answers /health and
	// /start, serves SSH with exactly our host key, and refuses a client key GitLab never issued
	// (the auth check goes to an unreachable GitLab, which must fail closed).
	register("gitlab-shell", func(t *T) {
		u := t.User("1001")
		t.Contains(t.RunAny("-version"), "gitlab-sshd v"+t.Want, "gitlab-sshd -version")
		dir, err := os.MkdirTemp(envOr("TMPDIR", ""), "gitlab-shell-smoke-")
		if err != nil {
			t.Fatalf("temp dir: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(dir) })
		_ = os.Chmod(dir, 0o755)
		hostPub, hostPriv, _ := ed25519.GenerateKey(rand.Reader)
		blk, err := ssh.MarshalPrivateKey(hostPriv, "")
		if err != nil {
			t.Fatalf("host key: %v", err)
		}
		cfg := `user: git
gitlab_url: "http://127.0.0.1:1"
secret: "smoke-secret-0123456789abcdef"
log_format: json
sshd:
  listen: "[::]:2222"
  web_listen: "0.0.0.0:9122"
  host_key_files:
    - /etc/gitlab-shell/ssh_host_ed25519_key
`
		for name, body := range map[string][]byte{"config.yml": []byte(cfg), "ssh_host_ed25519_key": pem.EncodeToMemory(blk)} {
			if err := os.WriteFile(filepath.Join(dir, name), body, 0o644); err != nil {
				t.Fatalf("%v", err)
			}
		}
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", dir + ":/etc/gitlab-shell:ro",
			"-p", "127.0.0.1:12222:2222", "-p", "127.0.0.1:19122:9122"})
		t.waitFor(30, time.Second, name, "gitlab-sshd never reported ready on /start", func() bool {
			if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
				t.Fatalf("gitlab-sshd exited\n%s", t.Logs(name))
			}
			c, _ := Get("http://127.0.0.1:19122/start")
			return c == 200
		})
		if c, _ := Get("http://127.0.0.1:19122/health"); c != 200 {
			t.Fatalf("/health answered %d", c)
		}
		want, _ := ssh.NewPublicKey(hostPub)
		_, clientPriv, _ := ed25519.GenerateKey(rand.Reader)
		signer, _ := ssh.NewSignerFromKey(clientPriv)
		var seen ssh.PublicKey
		conf := &ssh.ClientConfig{User: "git", Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)}, Timeout: 10 * time.Second,
			HostKeyCallback: func(_ string, _ net.Addr, k ssh.PublicKey) error { seen = k; return nil }}
		_, err = ssh.Dial("tcp", "127.0.0.1:12222", conf)
		if seen == nil || !bytes.Equal(seen.Marshal(), want.Marshal()) {
			t.Fatalf("the SSH host key is not the one we mounted (err %v)\n%s", err, t.Logs(name))
		}
		if err == nil || !strings.Contains(err.Error(), "unable to authenticate") {
			t.Fatalf("an unknown client key was not refused: %v", err)
		}
		t.Logf("smoke test passed (gitlab-shell %s: read-only boot, /health and /start ready, SSH with the mounted host key, unknown key refused; user %s)", t.Want, u)
	})
}
