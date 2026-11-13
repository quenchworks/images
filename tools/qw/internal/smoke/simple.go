package smoke

import "regexp"

// Short smoke tests: the image runs as uid 1001 and its binary answers. The component's
// real test is its chart's kind gate.

// regexpNoStart: the container never got as far as running the binary.
var regexpNoStart = regexp.MustCompile(`(?i)exec format|not found|no such file`)

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
	register("trust-manager", func(t *T) {
		out := t.Run("--help")
		t.Contains(out, "trust-manager", "--help")
		t.Contains(out, "--trust-namespace", "--help")
		u := t.User("1001")
		t.Logf("smoke test passed (CLI responds, nonroot user: %s)", u)
	})
	for _, a := range []string{"victoriametrics-operator", "victoriametrics-operator-config-reloader"} {
		register(a, func(t *T) {
			u := t.User("1001")
			v := t.Run("-version")
			t.Logf("%s", v)
			t.WantVersionV(v)
			t.Logf("smoke test passed (%s %s: version, nonroot user %s)", t.App, t.Want, u)
		})
	}
	register("calico-key-cert-provisioner", func(t *T) {
		t.User("1001")
		// it needs a pod to file its CSR: it must start and refuse to run unconfigured
		out := t.RunAny()
		t.Logf("%s", out)
		if out == "" {
			t.Fatalf("no output from key-cert-provisioner")
		}
		if m := regexpNoStart.FindString(out); m != "" {
			t.Fatalf("binary did not start: %s", m)
		}
		t.Logf("smoke test passed")
	})
	register("calico-typha", func(t *T) {
		t.User("1001")
		out := t.RunEntryAny("/usr/bin/calico-typha", "--version")
		t.Logf("/usr/bin/calico-typha --version: %s", out)
		t.WantVersionV(out)
		t.Logf("smoke test passed")
	})
	register("kfp-api-server", func(t *T) {
		t.User("1001")
		t.Contains(t.RunEntryAny("/usr/bin/apiserver", "--help"), "-sampleconfig", "apiserver --help")
		if t.Want != "" {
			t.Contains(t.Docker("inspect", t.Image, "--format", "{{json .Config.Env}}"), "TAG_NAME="+t.Want, "image env")
		}
		t.Logf("smoke test passed")
	})
	register("secrets-store-csi-driver", func(t *T) {
		out := t.RunAny("--version")
		t.Logf("secrets-store-csi --version: %s", out)
		t.WantVersionV(out)
		for _, b := range []string{"/usr/bin/mount", "/usr/bin/umount"} {
			t.RunEntry(b, "--version")
		}
		t.Logf("smoke test passed")
	})
	register("ingress-nginx", func(t *T) {
		out := t.Run("--version")
		t.Logf("%s", out)
		t.Match(out, `(?i)Release|Controller|nginx`, "version banner")
		ver := t.Match(out, `(v?[0-9]+\.[0-9]+\.[0-9]+)`, "version")
		u := t.User("1001")
		t.Logf("smoke test passed (ingress-nginx %s, nonroot %s)", ver, u)
	})
	register("node-feature-discovery", func(t *T) {
		u := t.User("1001")
		for _, b := range []string{"nfd-master", "nfd-worker", "nfd-topology-updater", "nfd-gc"} {
			t.WantVersionV(t.RunEntry("/usr/bin/"+b, "-version"))
		}
		t.Logf("smoke test passed (node-feature-discovery %s: 4 binaries report the version, nonroot user %s)", t.Want, u)
	})
	register("postgres-exporter", func(t *T) {
		t.Logf("version check:\n%s", t.Run("--version"))
		u := t.User("1001")
		t.Logf("smoke test passed (nonroot user: %s)", u)
	})
}
