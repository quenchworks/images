package smoke

// Short smoke tests: the image runs as uid 1001 and its binary answers. The component's
// real test is its chart's kind gate.

// versionWant: nonroot, and `-version` (or args) succeeds and reports the expected version.
func versionWant(args ...string) func(*T) {
	return func(t *T) {
		u := t.User("1001")
		v := t.Run(args...)
		t.Logf("%s", v)
		t.WantVersion(v)
		t.Logf("smoke test passed (%s %s: version, nonroot user %s)", t.App, t.Want, u)
	}
}

// helpFlag: nonroot, and entry's help output (exit code ignored) lists flag.
func helpFlag(entry, helpArg, flag string) func(*T) {
	return func(t *T) {
		t.User("1001")
		out := t.RunEntryAny(entry, helpArg)
		t.Contains(out, flag, entry+" "+helpArg)
		t.Logf("smoke test passed")
	}
}

func init() {
	register("calico-csi", func(t *T) {
		t.Contains(t.RunEntryAny("/usr/bin/csi-driver", "-help"), "-endpoint", "csi-driver usage")
		t.Logf("smoke test passed")
	})
	register("calico-pod2daemon-flexvol", func(t *T) {
		out := t.RunEntryAny("/usr/local/bin/flexvol", "init")
		t.Logf("%s", out)
		t.Contains(out, `"status"`, "flexvol init status")
		t.RunEntry("/bin/bash", "-c", "test -x /usr/local/bin/flexvol.sh && command -v cp mv chmod")
		t.Logf("smoke test passed")
	})
	register("step-issuer", func(t *T) {
		t.Contains(t.Run("-help"), "-disable-approval-check", "step-issuer -help")
		u := t.User("1001")
		t.Logf("smoke test passed (flags parse, nonroot user: %s)", u)
	})
	register("clickhouse-operator", versionWant("-version"))
	register("clickhouse-operator-metrics-exporter", versionWant("-version"))
	register("nats-jetstream-controller", versionWant("-version"))
	register("envoy", func(t *T) {
		out := t.Run("--version")
		t.Logf("  %s", out)
		t.Match(out, `(?i)envoy version|/[0-9]+\.[0-9]+\.[0-9]+/`, "version banner")
		ver := t.Match(out, `([0-9]+\.[0-9]+\.[0-9]+)`, "version number")
		if ver == "0.0.0" {
			t.Fatalf("version not stamped: '%s'", ver)
		}
		u := t.User("1001")
		t.Logf("smoke test passed (envoy %s, nonroot %s)", ver, u)
	})
	register("kfp-driver", helpFlag("/usr/bin/driver", "--help", "-mlmd_server_address"))
	register("kfp-persistence-agent", helpFlag("/usr/bin/persistence_agent", "--help", "-ttlSecondsAfterWorkflowFinish"))
	register("kfp-scheduled-workflow-controller", helpFlag("/usr/bin/controller", "--help", "-recurringRunResyncIntervalSeconds"))
}
