package smoke

func init() {
	// Strix needs an LLM key and a Docker API for a real scan, so the smoke test covers what runs without them: the
	// CLI reports its version and help on a read-only root filesystem (home on tmpfs), and the Go TUI sidecar the
	// hatch hook built from source is present and executes.
	register("strix", func(t *T) {
		v := t.Run("--version")
		t.Logf("strix --version: %s", v)
		t.WantVersion(v)
		out, err := t.RunFlags([]string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--tmpfs", "/home/strix:rw,uid=1001,gid=1001"}, "--help")
		if err != nil {
			t.Fatalf("strix --help on a read-only rootfs: %v\n%s", err, out)
		}
		t.ContainsFold(out, "strix", "help text")
		tui := t.RunEntry("/opt/strix/venv/bin/python", "-c",
			"import os,strix,subprocess;p=os.path.join(os.path.dirname(strix.__file__),'bin','strix-tui');print(os.access(p,os.X_OK))")
		t.Contains(tui, "True", "TUI sidecar executable")
		t.Logf("smoke test passed (strix %s: version, help on a read-only rootfs, TUI sidecar; user %s)", t.Want, t.User("1001"))
	})
}
