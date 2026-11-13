package smoke

func init() {
	register("cosign", func(t *T) {
		out := t.Run("version")
		t.Logf("cosign version:\n%s", out)
		// stamped from the tag, not the default "devel"/"unknown"
		t.Match(out, `GitVersion:\s*v[0-9]`, "version not stamped")
		t.WantVersion(out)
		// a real subcommand resolves: the binary is functional, not just --help
		t.Run("verify", "--help")
		u := t.User("1001")
		t.Logf("smoke test passed (nonroot user: %s)", u)
	})
}
