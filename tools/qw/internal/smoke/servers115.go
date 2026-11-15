package smoke

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// restDo sends one rest-server API request with basic auth and returns the status and body.
func restDo(method, url, user, pass, body string) (int, string) {
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		return 0, err.Error()
	}
	if user != "" {
		req.SetBasicAuth(user, pass)
	}
	return insecureDo(req)
}

func init() {
	// On a read-only rootfs with /data on a volume and users from an htpasswd file: anonymous and
	// wrong-password requests get 401, the user creates a repository and stores its config, and the
	// config reads back after a restart on the same volume.
	register("rest-server", func(t *T) {
		const base = "http://127.0.0.1:18000"
		const user, pw = "smoke", "Smoke-Pass-2026"
		u := t.User("1001")
		t.Contains(t.RunAny("--version"), t.Want, "rest-server --version")
		dir := t.TempDir()
		h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.MinCost)
		if err != nil {
			t.Fatalf("bcrypt: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".htpasswd"), []byte(user+":"+string(h)+"\n"), 0o644); err != nil {
			t.Fatalf("htpasswd: %v", err)
		}
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		start := func(name string) {
			t.Start(name, []string{"--read-only", "-v", vol + ":/data", "-v", dir + ":/auth:ro", "-p", "127.0.0.1:18000:8000"},
				"--path", "/data", "--listen", ":8000", "--htpasswd-file", "/auth/.htpasswd")
			t.waitFor(60, time.Second, name, "rest-server never answered", func() bool {
				if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
					t.Fatalf("rest-server exited\n%s", t.Logs(name))
				}
				code, _ := restDo("GET", base+"/", "", "", "")
				return code == http.StatusUnauthorized
			})
		}
		name := t.Name("")
		start(name)
		if code, _ := restDo("POST", base+"/repo/?create=true", user, "wrong-password-1", ""); code != http.StatusUnauthorized {
			t.Fatalf("a wrong password got %d, want 401", code)
		}
		if code, body := restDo("POST", base+"/repo/?create=true", user, pw, ""); code != http.StatusOK {
			t.Fatalf("create repo: %d %s\n%s", code, body, t.Logs(name))
		}
		if code, body := restDo("POST", base+"/repo/config", user, pw, "quench smoke config"); code != http.StatusOK {
			t.Fatalf("save config: %d %s\n%s", code, body, t.Logs(name))
		}
		docker("rm", "-f", name)
		start(t.Name("again"))
		if code, body := restDo("GET", base+"/repo/config", user, pw, ""); code != http.StatusOK || body != "quench smoke config" {
			t.Fatalf("config after a restart: %d %q (the repository is gone?)", code, body)
		}
		t.Logf("smoke test passed (rest-server %s: read-only boot, htpasswd auth enforced, repository created and its config kept across a restart; user %s)", t.Want, u)
	})
}

func init() {
	// On a read-only rootfs: restic initialises an encrypted repository in a bind mount, backs up a file,
	// restores it, and refuses the repository with a wrong password.
	register("restic", func(t *T) {
		u := t.User("1001")
		t.Contains(t.RunAny("version"), t.Want, "restic version")
		// the source is a file inside the image: a bind-mounted host file reads back EIO through Docker
		// Desktop's file share when restic opens it
		dir := t.TempDir()
		run := func(pw string, args ...string) (string, int) {
			flags := []string{"--read-only", "--tmpfs", "/home/restic:rw,uid=1001,gid=1001", "--tmpfs", "/tmp:rw,mode=1777", "-v", dir + ":/w", "-e", "RESTIC_PASSWORD=" + pw, "-e", "RESTIC_REPOSITORY=/w/repo"}
			return t.RunAnyFor(2*time.Minute, flags, args...)
		}
		const pw = "Smoke-Pass-2026"
		for _, step := range [][]string{{"init"}, {"backup", "/etc/os-release"}, {"restore", "latest", "--target", "/w/out"}} {
			if out, rc := run(pw, step...); rc != 0 {
				t.Fatalf("restic %s failed (%d):\n%s", step[0], rc, out)
			}
		}
		got, err := os.ReadFile(filepath.Join(dir, "out", "etc", "os-release"))
		if err != nil || !strings.Contains(string(got), "ID=") {
			t.Fatalf("restored file: %q %v", got, err)
		}
		if out, rc := run("wrong-password-1", "snapshots"); rc == 0 {
			t.Fatalf("a wrong password opened the repository:\n%s", out)
		}
		t.Logf("smoke test passed (restic %s: read-only rootfs, encrypted repository initialised, file backed up and restored, wrong password refused; user %s)", t.Want, u)
	})
}
