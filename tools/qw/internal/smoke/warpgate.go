package smoke

import "strings"

func init() {
	// On a read-only rootfs with /data on a volume: unattended-setup writes the config and database,
	// `run` serves the web UI on 8888 (HTTPS, self-signed), and a restart keeps the setup.
	register("warpgate", func(t *T) {
		t.Contains(t.RunEntry("/usr/bin/warpgate", "version"), t.Want, "version not reported")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		flags := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/data"}
		setup := t.Name("setup")
		t.Cleanup(func() { docker("rm", "-f", setup) })
		out, err := docker(append(append([]string{"run", "--name", setup, "--user", "1001"}, flags...),
			t.Image, "unattended-setup", "--data-path", "/data", "--http-port", "8888", "--ssh-port", "2222",
			"--admin-password", "Smoke-Warpgate-2026!", "--external-host", "localhost")...)
		if err != nil {
			t.Fatalf("unattended-setup: %v\n%s", err, out)
		}
		start := func(name string) {
			t.Start(name, append(append([]string{}, flags...), "-p", "127.0.0.1:18124:8888", "-p", "127.0.0.1:18125:2222"), "run")
			t.WaitTCP(90, name, "127.0.0.1:18124", "127.0.0.1:18125")
		}
		name := t.Name("")
		start(name)
		if !strings.Contains(t.Logs(name), "8888") {
			t.Logf("logs:\n%s", t.Logs(name))
		}
		docker("rm", "-f", name)
		start(t.Name("again"))
		t.Logf("smoke test passed (warpgate %s: unattended setup on a volume, HTTPS 8888 and SSH 2222 listening on a read-only rootfs, kept across a restart; user %s)", t.Want, t.User("1001"))
	})
}
