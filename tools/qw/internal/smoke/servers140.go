package smoke

import (
	"net/http"
	"strings"
)

func init() {
	// On a read-only rootfs with our memcached as the store: the web UI and /version answer, a message
	// that is not PGP armor is refused, the image's own yopass CLI encrypts a secret through the API and
	// decrypts it once, and a second read of the one-time secret fails.
	register("yopass", func(t *T) {
		const base = "http://127.0.0.1:18092"
		const secret = "quench smoke secret 2026"
		u := t.User("1001")
		net := t.Name("net")
		t.Docker("network", "create", net)
		t.Cleanup(func() { docker("network", "rm", net) })
		mc := t.Name("memcached")
		t.Cleanup(func() { docker("rm", "-f", mc) })
		t.Docker("run", "-d", "--name", mc, "--network", net, "ghcr.io/quenchworks/images/memcached:1.6.45")
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--network", net, "-e", "MEMCACHED=" + mc + ":11211", "-p", "127.0.0.1:18092:1337"})
		t.WaitHTTP(base+"/health", 60, name)
		_, page := Get(base + "/")
		t.Contains(page, `<div id="root"`, "the web UI index")
		_, ver := Get(base + "/version")
		if t.Want != "" {
			t.Contains(ver, t.Want, "/version")
		}
		if c, body := restDo("POST", base+"/create/secret", "", "", `{"message":"plain text","expiration":3600,"one_time":true}`); c != http.StatusBadRequest {
			t.Fatalf("a plain-text message got %d %s, want 400", c, body)
		}
		api := "http://" + name + ":1337"
		cli := []string{"--network", net, "--entrypoint", "/usr/bin/yopass"}
		out, err := t.RunInputFlags(cli, secret, "--api", api, "--url", api, "--expiration", "1h", "--one-time")
		link := strings.TrimSpace(out)
		if err != nil || !strings.HasPrefix(link, api) {
			t.Fatalf("yopass CLI store: %v %q\n%s", err, out, t.Logs(name))
		}
		if got, err := t.RunFlags(cli, "--api", api, "--url", api, "--decrypt", link); err != nil || strings.TrimSpace(got) != secret {
			t.Fatalf("yopass CLI decrypt: %v %q", err, got)
		}
		if got, err := t.RunFlags(cli, "--api", api, "--url", api, "--decrypt", link); err == nil || strings.Contains(got, secret) {
			t.Fatalf("a second read of the one-time secret worked: %q", got)
		}
		t.Logf("smoke test passed (yopass %s: read-only boot on memcached, web UI and version served, plain text refused, CLI secret stored and read once, second read refused; user %s)", t.Want, u)
	})
}
