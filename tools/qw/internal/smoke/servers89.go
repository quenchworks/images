package smoke

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func init() {
	// On a read-only rootfs: /healthcheck, the first user registers through the CSRF-protected
	// form, a gist is created by `git push` over HTTP (the server runs git for it), cloned back
	// with the host's git, and its page renders.
	register("opengist", func(t *T) {
		const base = "http://127.0.0.1:16157"
		const pw = "Smoke-Pass-2026"
		u := t.User("1001")
		t.Contains(t.RunAny("version"), t.Want, "opengist version")
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--tmpfs", "/opengist:rw,uid=1001,gid=1001", "-p", "127.0.0.1:16157:6157"})
		t.waitFor(60, time.Second, name, "opengist never answered /healthcheck", func() bool {
			if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
				t.Fatalf("opengist exited\n%s", t.Logs(name))
			}
			c, _ := Get(base + "/healthcheck")
			return c == 200
		})
		jar, _ := cookiejar.New(nil)
		cl := &http.Client{Jar: jar, Timeout: 15 * time.Second}
		resp, err := cl.Get(base + "/-/register")
		if err != nil {
			t.Fatalf("GET /-/register: %v", err)
		}
		page, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		csrf := t.Match(string(page), `name="_csrf" value="([^"]+)"`, "CSRF token on the register form")
		resp, err = cl.PostForm(base+"/-/register", url.Values{"username": {"smoke"}, "password": {pw}, "_csrf": {csrf}})
		if err != nil || resp.StatusCode >= 400 {
			t.Fatalf("register: %v %v", err, resp)
		}
		resp.Body.Close()

		dir := t.TempDir()
		git := func(d string, args ...string) {
			c := exec.Command("git", append([]string{"-c", "user.name=smoke", "-c", "user.email=smoke@example.com", "-c", "init.defaultBranch=master"}, args...)...)
			c.Dir = d
			if out, err := c.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s\n%s", args, err, out, t.Logs(name))
			}
		}
		src := filepath.Join(dir, "src")
		os.MkdirAll(src, 0o755)
		os.WriteFile(filepath.Join(src, "hello.txt"), []byte("quench gist\n"), 0o644)
		git(src, "init", "-q")
		git(src, "add", "hello.txt")
		git(src, "commit", "-qm", "smoke")
		remote := "http://smoke:" + pw + "@127.0.0.1:16157/smoke/smoke-gist"
		git(src, "push", "-q", remote, "master")
		git(dir, "clone", "-q", remote, filepath.Join(dir, "back"))
		if b, _ := os.ReadFile(filepath.Join(dir, "back", "hello.txt")); string(b) != "quench gist\n" {
			t.Fatalf("cloned gist content %q", b)
		}
		resp, err = cl.Get(base + "/smoke/smoke-gist")
		if err != nil {
			t.Fatalf("gist page: %v", err)
		}
		gp, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || !strings.Contains(string(gp), "hello.txt") {
			t.Fatalf("gist page: %d", resp.StatusCode)
		}
		if regexp.MustCompile(`(?i)permission denied|read-only file system`).MatchString(t.Logs(name)) {
			t.Fatalf("errors on a read-only rootfs\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed (opengist %s: read-only boot, register, gist created by git push and cloned back, page renders; user %s)", t.Want, u)
	})
}
