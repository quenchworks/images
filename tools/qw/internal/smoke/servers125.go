package smoke

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func init() {
	// On a read-only rootfs: hugo builds a site from a git repository in a bind mount, renders a
	// content page through a layout, and fills .GitInfo from the commit (the image's git, no shell).
	register("hugo", func(t *T) {
		u := t.User("1001")
		t.Contains(t.RunAny("version"), "v"+t.Want, "hugo version")
		dir := t.TempDir()
		files := map[string]string{
			"hugo.toml":         "baseURL = 'https://example.org/'\ntitle = 'Quench Smoke'\nenableGitInfo = true\ndisableKinds = ['taxonomy', 'term', 'RSS', 'sitemap', 'robotsTXT', '404']\n",
			"content/_index.md": "---\ntitle: Home\n---\nHello from **quench-hugo**.\n",
			"layouts/home.html": "<h1>{{ .Site.Title }}</h1>{{ .Content }}<p id=\"git\">{{ with .GitInfo }}{{ .Subject }}{{ end }}</p>\n",
		}
		for n, c := range files {
			p := filepath.Join(dir, n)
			_ = os.MkdirAll(filepath.Dir(p), 0o777)
			if err := os.WriteFile(p, []byte(c), 0o644); err != nil {
				t.Fatalf("write %s: %v", n, err)
			}
		}
		for _, args := range [][]string{{"init", "-q"}, {"add", "."}, {"commit", "-q", "-m", "quench smoke commit"}} {
			c := exec.Command("git", append([]string{"-c", "user.name=smoke", "-c", "user.email=smoke@quench-works.com", "-c", "init.defaultBranch=main"}, args...)...)
			c.Dir = dir
			if out, err := c.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}
		// the repo is owned by the host user, not uid 1001: git refuses it without safe.directory
		flags := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", dir + ":/w",
			"-e", "GIT_CONFIG_COUNT=1", "-e", "GIT_CONFIG_KEY_0=safe.directory", "-e", "GIT_CONFIG_VALUE_0=*"}
		if out, err := t.RunInputFlags(flags, "", "--source", "/w", "--destination", "/w/public", "--noBuildLock"); err != nil {
			t.Fatalf("hugo build: %v\n%s", err, out)
		}
		got, err := os.ReadFile(filepath.Join(dir, "public", "index.html"))
		page := string(got)
		if err != nil || !strings.Contains(page, "<h1>Quench Smoke</h1>") || !strings.Contains(page, "<strong>quench-hugo</strong>") || !strings.Contains(page, `<p id="git">quench smoke commit</p>`) {
			t.Fatalf("built page: %v\n%s", err, page)
		}
		t.Logf("smoke test passed (hugo %s: site built from a git repo on a read-only rootfs, Markdown rendered through a layout, .GitInfo filled by git; user %s)", t.Want, u)
	})
}
