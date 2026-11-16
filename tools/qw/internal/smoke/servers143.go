package smoke

import (
	"net/http"
	"strings"
	"time"
)

func init() {
	// On a read-only rootfs with the file cache on a tmpfs: the bridge list includes DemoBridge, DemoBridge renders
	// as Atom and JSON (no network needed), only index.php runs, the enabled-bridges list is enforced, and HTTP basic
	// auth from the environment refuses an anonymous request.
	register("rss-bridge", func(t *T) {
		const base = "http://127.0.0.1:18095"
		const demo = base + "/?action=display&bridge=DemoBridge&context=testCheckbox&format="
		u := t.User("1001")
		start := func(name string, env ...string) {
			flags := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--tmpfs", "/var/cache/rss-bridge:rw,uid=1001,gid=1001",
				"-p", "127.0.0.1:18095:8080"}
			for _, e := range env {
				flags = append(flags, "-e", e)
			}
			t.Start(name, flags)
			// with auth enabled / answers 401: either way the server is up
			t.waitFor(60, time.Second, name, "rss-bridge never served /", func() bool {
				c, _ := Get(base + "/")
				return c == http.StatusOK || c == http.StatusUnauthorized
			})
		}
		name := t.Name("")
		start(name, "RSSBRIDGE_system_enabled_bridges=DemoBridge,GithubIssueBridge")
		_, list := Get(base + "/?action=list")
		t.Contains(list, `"DemoBridge"`, "the bridge list")
		_, atom := Get(demo + "Atom")
		t.Contains(atom, "<feed", "the Atom feed")
		t.Contains(atom, "Awesome content", "the Atom feed item")
		_, js := Get(demo + "Json")
		t.Contains(js, "Lalala", "the JSON feed item")
		if c, b := Get(base + "/?action=display&bridge=WikipediaBridge&format=Atom"); c < 400 || strings.Contains(b, "<feed") {
			t.Fatalf("a bridge outside enabled_bridges answered %d", c)
		}
		for _, p := range []string{"/config.default.ini.php", "/lib/bootstrap.php", "/bridges/DemoBridge.php"} {
			if c, _ := Get(base + p); c != http.StatusForbidden {
				t.Fatalf("%s answered %d, want 403", p, c)
			}
		}
		docker("rm", "-f", name)
		start(t.Name("auth"), "RSSBRIDGE_authentication_enable=true", "RSSBRIDGE_authentication_username=smoke",
			"RSSBRIDGE_authentication_password=Smoke-Bridge-2026")
		if c, _ := Get(demo + "Atom"); c != http.StatusUnauthorized {
			t.Fatalf("an anonymous request got %d with auth enabled, want 401", c)
		}
		if c, b := restDo("GET", demo+"Atom", "smoke", "Smoke-Bridge-2026", ""); c != http.StatusOK || !strings.Contains(b, "<feed") {
			t.Fatalf("the right password got %d", c)
		}
		t.Logf("smoke test passed (rss-bridge %s: read-only boot, bridge list, DemoBridge as Atom and JSON, enabled_bridges enforced, only index.php runs, basic auth from the environment; user %s)", t.Want, u)
	})
}
