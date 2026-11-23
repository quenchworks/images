package smoke

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func init() {
	// On a read-only root with the data volume: soft serve starts as uid 1001, the HTTP transport takes a push of a
	// new repository (anonymous read-write for the test), the repository clones back with its commit, and the data
	// survives a restart.
	register("soft-serve", func(t *T) {
		const base = "http://127.0.0.1:18130"
		u := t.User("1001")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", "-f", vol) })
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/soft-serve", "-p", "127.0.0.1:18130:23232",
			"-e", "SOFT_SERVE_ANON_ACCESS=read-write", "-e", "SOFT_SERVE_HTTP_PUBLIC_URL=" + base})
		t.waitFor(60, time.Second, name, "the HTTP transport did not answer", func() bool {
			c, _ := Get(base + "/")
			return c > 0
		})
		git := func(dir string, args ...string) string {
			c := exec.Command("git", args...)
			c.Dir = dir
			c.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_AUTHOR_NAME=quench", "GIT_AUTHOR_EMAIL=q@example.test",
				"GIT_COMMITTER_NAME=quench", "GIT_COMMITTER_EMAIL=q@example.test")
			out, err := c.CombinedOutput()
			if err != nil {
				t.Fatalf("git %s: %v\n%s\n%s", strings.Join(args, " "), err, out, t.Logs(name))
			}
			return string(out)
		}
		src := t.TempDir()
		git(src, "init", "-q", "-b", "main")
		if err := os.WriteFile(filepath.Join(src, "README.md"), []byte("quench smoke\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		git(src, "add", "README.md")
		git(src, "commit", "-q", "-m", "quench smoke commit")
		git(src, "push", "-q", base+"/smoke.git", "main")
		check := func() {
			dst := filepath.Join(t.TempDir(), "clone")
			git(filepath.Dir(dst), "clone", "-q", base+"/smoke.git", dst)
			t.Contains(git(dst, "log", "--format=%s", "-1"), "quench smoke commit", "cloned commit")
		}
		check()
		t.Docker("restart", name)
		t.waitFor(60, time.Second, name, "the HTTP transport did not come back", func() bool {
			c, _ := Get(base + "/")
			return c > 0
		})
		check()
		if c, _ := Get(base + "/smoke.git/info/refs?service=git-upload-pack"); c != http.StatusOK {
			t.Fatalf("smart HTTP refs: %d", c)
		}
		t.Logf("smoke test passed (soft-serve %s: uid 1001 on a read-only root, a repository pushed over HTTP and cloned back, kept across a restart; user %s)", t.Want, u)
	})
}
