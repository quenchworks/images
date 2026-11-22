package smoke

func init() {
	// Without a cluster the operator cannot reconcile, so the test covers the binary itself: the image label carries
	// the release, the root help lists the roles the one binary serves (the operator, its webhook server and the
	// sidecar that runs inside Scylla pods), and the operator subcommand parses its flags.
	register("scylla-operator", func(t *T) {
		t.LabelVersion()
		u := t.User("1001")
		root := t.Run("--help")
		for _, sub := range []string{"operator", "run-webhook-server", "sidecar", "node-setup-daemon"} {
			t.Contains(root, sub, "subcommand "+sub)
		}
		t.Contains(t.Run("operator", "--help"), "--", "operator flags")
		t.Logf("version: %s", t.Run("version"))
		t.Logf("smoke test passed (scylla-operator %s: image label, operator/webhook/sidecar/node-setup roles, operator flags; user %s)", t.Want, u)
	})
}
