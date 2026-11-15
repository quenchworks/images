package smoke

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"
)

func init() {
	// On a read-only rootfs with /app and /repository on volumes: a filesystem repository is
	// created and a snapshot of /etc/ssl taken and listed with the CLI, then the repository server
	// starts on the same volumes and answers its API (repo status: connected) with basic auth,
	// refuses a wrong password, and serves the web UI.
	register("kopia", func(t *T) {
		const base = "http://127.0.0.1:15151"
		const repoPW = "smoke-repo-pass-2026"
		const uiPW = "smoke-ui-pass-2026"
		u := t.User("1001")
		t.Contains(t.RunAny("--version"), t.Want, "kopia --version")
		app, repo := t.Name("app"), t.Name("repo")
		for _, v := range []string{app, repo} {
			v := v
			t.Docker("volume", "create", v)
			t.Cleanup(func() { docker("volume", "rm", v) })
		}
		// empty named volumes are seeded from the image's /app and /repository (1001-owned)
		vols := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", app + ":/app", "-v", repo + ":/repository", "-e", "KOPIA_PASSWORD=" + repoPW}
		run := func(args ...string) string {
			out, err := t.RunFlags(vols, args...)
			if err != nil {
				t.Fatalf("kopia %s: %v\n%s", strings.Join(args, " "), err, out)
			}
			return out
		}
		run("repository", "create", "filesystem", "--path=/repository")
		run("snapshot", "create", "/etc/ssl") // readable by uid 1001 (unlike /etc/shadow)
		t.Contains(run("snapshot", "list", "/etc/ssl"), "/etc/ssl", "snapshot list")

		name := t.Name("")
		t.Start(name, append(vols, "-p", "127.0.0.1:15151:51515"),
			"server", "start", "--insecure", "--address=0.0.0.0:51515",
			"--server-username=admin", "--server-password="+uiPW)
		// the API wants the browser's flow: an authenticated UI load sets the session cookie and
		// embeds the CSRF token (meta kopia-csrf-token), then API calls send both
		jar, _ := cookiejar.New(nil)
		cl := &http.Client{Jar: jar, Timeout: 10 * time.Second}
		get := func(path, pw, csrf string) (int, string) {
			req, _ := http.NewRequest("GET", base+path, nil)
			req.SetBasicAuth("admin", pw)
			if csrf != "" {
				req.Header.Set("X-Kopia-Csrf-Token", csrf)
			}
			resp, err := cl.Do(req)
			if err != nil {
				return 0, err.Error()
			}
			defer resp.Body.Close()
			b, _ := io.ReadAll(resp.Body)
			return resp.StatusCode, string(b)
		}
		var page string
		t.waitFor(60, time.Second, name, "the kopia server never served its UI", func() bool {
			if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
				t.Fatalf("kopia server exited\n%s", t.Logs(name))
			}
			c, b := get("/", uiPW, "")
			page = b
			return c == 200
		})
		csrf := t.Match(page, `name="kopia-csrf-token" content="([^"]+)"`, "CSRF token in the web UI")
		c, status := get("/api/v1/repo/status", uiPW, csrf)
		if c != 200 {
			t.Fatalf("repo status answered %d: %s\n%s", c, status, t.Logs(name))
		}
		t.Match(status, `"connected":\s*(true)`, "repository status over the API: connected")
		// a fresh client (no session cookie) with a wrong password must be refused
		bad, _ := http.NewRequest("GET", base+"/", nil)
		bad.SetBasicAuth("admin", "wrong-password-1")
		if resp, err := http.DefaultClient.Do(bad); err != nil || resp.StatusCode != 401 {
			t.Fatalf("a wrong password was not refused with 401: %v %v", err, resp)
		} else {
			resp.Body.Close()
		}
		t.Logf("smoke test passed (kopia %s: read-only, repository created, /etc/ssl snapshot listed, server API connected with basic auth, wrong password refused, web UI served; user %s)", t.Want, u)
	})
}
