package smoke

import (
	"strings"
)

func init() {
	// On a read-only rootfs with /gotosocial/storage on a volume: the server runs the migrations, the admin CLI
	// (inside the running container: it needs the instance application the first start creates) creates,
	// confirms and promotes an account, the instance API shows the version, and after a restart WebFinger
	// still finds the account.
	register("gotosocial", func(t *T) {
		const base = "http://127.0.0.1:18085"
		const host = "gts.quench.test"
		u := t.User("1001")
		t.Contains(t.RunAny("--version"), t.Want, "gotosocial --version")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		flags := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/gotosocial/storage", "-e", "GTS_HOST=" + host}
		start := func(name string) {
			t.Start(name, append(flags, "-p", "127.0.0.1:18085:8080"))
			t.WaitHTTP(base+"/api/v1/instance", 120, name)
		}
		name := t.Name("")
		start(name)
		// the admin CLI needs the instance application the first server start creates
		create := "/gotosocial/gotosocial admin account create --username smoke --email smoke@quench.test --password 'Smoke-Pass-2026-long!' &&" +
			" /gotosocial/gotosocial admin account confirm --username smoke && /gotosocial/gotosocial admin account promote --username smoke"
		if out, err := docker("exec", name, "/bin/sh", "-c", create); err != nil {
			t.Fatalf("admin account create failed: %v\n%s", err, out)
		}
		t.Contains(t.WaitHTTP(base+"/api/v1/instance", 5, name), t.Want, "the instance API version")
		docker("rm", "-f", name)
		start(t.Name("again"))
		wf := t.WaitHTTP(base+"/.well-known/webfinger?resource=acct:smoke@"+host, 10, t.Name("again"))
		if !strings.Contains(wf, "acct:smoke@"+host) {
			t.Fatalf("webfinger after a restart: %q", wf)
		}
		t.Logf("smoke test passed (gotosocial %s: read-only boot, admin created by the CLI, instance API, the account found by WebFinger after a restart; user %s)", t.Want, u)
	})
}
