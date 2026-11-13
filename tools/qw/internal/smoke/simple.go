package smoke

import (
	"regexp"
	"strings"
)

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
	for _, c := range []string{"acmesolver", "cainjector", "controller", "webhook"} {
		register("cert-manager-"+c, func(t *T) {
			out := t.Run("--help")
			t.ContainsFold(out, "Usage:", "--help")
			t.ContainsFold(out, c, "--help")
			u := t.User("1001")
			t.Logf("smoke test passed (%s CLI responds, nonroot user: %s)", c, u)
		})
	}
	register("dragonfly-operator", func(t *T) {
		u := t.User("1001")
		t.LabelVersion()
		// upstream's Version is a const, "source": the label carries the release
		if v := t.Run("-version"); v != "source" {
			t.Fatalf("unexpected -version output: %s", v)
		}
		t.Contains(t.RunAny("--help"), "-dragonfly-image", "--help")
		t.Logf("smoke test passed (dragonfly-operator %s: CLI, image label, nonroot user %s)", t.Want, u)
	})
	register("opensearch-operator", func(t *T) {
		u := t.User("1001")
		t.LabelVersion()
		h := t.RunAny("--help")
		t.Contains(h, "-watch-namespace", "--help")
		t.Contains(h, "-enable-webhooks", "--help")
		t.Logf("smoke test passed (opensearch-operator %s: CLI, image label, nonroot user %s)", t.Want, u)
	})
	register("kfp-launcher", func(t *T) {
		t.User("1001")
		t.Contains(t.RunEntryAny("/usr/bin/launcher-v2", "--help"), "-copy", "launcher-v2 --help")
		// every task pod runs this image as an init container that copies the binary out
		if out, err := t.RunFlags([]string{"--tmpfs", "/kfp-launcher:uid=1001,exec", "--entrypoint", "/usr/bin/launcher-v2"}, "--copy", "/kfp-launcher/launch"); err != nil {
			t.Fatalf("launcher-v2 --copy failed: %v %s", err, out)
		}
		t.Logf("smoke test passed")
	})
	register("cephcsi", func(t *T) {
		out := t.Run("--version")
		t.Logf("%s", out)
		ver := t.Match(out, `(?m)^Cephcsi Version: v([0-9.]*)`, "cephcsi version")
		if t.Want != "" && ver != t.Want {
			t.Fatalf("expected cephcsi %s, got '%s'", t.Want, ver)
		}
		t.Logf("%s", t.RunEntry("/bin/bash", "-c", `set -e
for b in rbd ceph ceph-fuse mount umount blkid blockdev fsck findmnt lsblk mkfs.ext4 e2fsck resize2fs mkfs.xfs xfs_growfs cryptsetup modprobe mount.nfs; do command -v $b >/dev/null || { echo "missing $b"; exit 1; }; done
echo tools present`))
		t.Logf("smoke test passed")
	})
	register("ceph-csi-operator", func(t *T) {
		t.User("1001")
		// the manager panics without OPERATOR_NAMESPACE, even for --help
		env := []string{"-e", "OPERATOR_NAMESPACE=smoke"}
		help, _ := t.RunFlags(env, "--help")
		t.Contains(help, "leader-elect", "manager --help")
		out, _ := t.RunFlags(env)
		t.Match(out, `(?i)kubeconfig|in-cluster|KUBERNETES_SERVICE_HOST|unable to get kubeconfig`, "start without an API server")
		t.Logf("manager runs and stops on the missing API server\nsmoke test passed")
	})
	register("grafana-operator", func(t *T) {
		u := t.User("1001")
		t.Contains(t.RunAny("--help"), "--leader-elect", "--help")
		// no API server: it logs its version, then fails to load a kubeconfig
		t.WantVersionV(t.RunAny())
		t.Logf("smoke test passed (grafana-operator %s: CLI, version in startup log, nonroot user %s)", t.Want, u)
	})
	register("vitess-operator", func(t *T) {
		u := t.User("1001")
		// without a cluster it logs its version and exits on the missing kubeconfig
		t.Contains(t.RunAny(), "Operator Version: "+t.Want, "startup log")
		t.Contains(t.RunAny("--help"), "cache_invalidate_interval", "--help")
		t.Logf("smoke test passed (vitess-operator %s: startup version, CLI, nonroot user %s)", t.Want, u)
	})
	register("kueue", func(t *T) {
		u := t.User("1001")
		t.Contains(t.RunAny("--help"), "-config", "--help")
		t.WantVersionV(t.RunAny())
		t.Logf("smoke test passed (kueue %s: CLI, version in startup log, nonroot user %s)", t.Want, u)
	})
	register("calico-cni", func(t *T) {
		// the CNI spec's VERSION handshake on every plugin
		for _, b := range []string{"calico", "calico-ipam", "bandwidth", "host-local", "loopback", "portmap", "tuning", "flannel"} {
			out, _ := t.RunFlags([]string{"-e", "CNI_COMMAND=VERSION", "--entrypoint", "/opt/cni/bin/" + b})
			t.Contains(out, `"supportedVersions"`, b+" CNI VERSION reply")
			t.Logf("%s: ok", b)
		}
		v := t.RunEntryAny("/opt/cni/bin/calico", "-v")
		t.Logf("calico -v: %s", v)
		t.WantVersionV(v)
		t.Logf("smoke test passed")
	})
	register("emissary-apiext", func(t *T) {
		// outside a cluster it stops at its in-cluster config, after its start line
		out := t.RunAny()
		t.Contains(out, "starting Emissary-ingress apiext webhook conversion server", "start line")
		ver := t.Match(out, `"version": ?"([0-9]+\.[0-9]+\.[0-9]+[^"]*)"`, "version")
		if strings.HasPrefix(ver, "MISSING") {
			t.Fatalf("version not read: '%s'", ver)
		}
		if t.Want != "" && ver != t.Want {
			t.Fatalf("version %s, expected %s", ver, t.Want)
		}
		u := t.User("8888")
		t.Logf("smoke test passed (version %s, user %s)", ver, u)
	})
	register("hadolint", func(t *T) {
		out, _ := t.RunInput("FROM ubuntu\nRUN apt-get update && apt-get install -y curl\n", "--no-color", "-")
		t.Contains(out, "DL3006", "known-bad input")
		t.Contains(out, "DL3008", "known-bad input")
		f := strings.Fields(t.Run("--version"))
		v := f[len(f)-1]
		t.Logf("reported version: %s", v)
		if t.Want != "" && v != t.Want {
			t.Fatalf("expected %s", t.Want)
		}
		u := t.User("1001")
		t.Logf("smoke test passed (hadolint %s, nonroot user: %s, known-bad input flagged)", v, u)
	})
	register("shellcheck", func(t *T) {
		out, _ := t.RunInput("#!/bin/sh\necho $1\n", "-f", "gcc", "-")
		t.Contains(out, "SC2086", "known-bad input")
		v := t.Match(t.Run("--version"), `(?m)^version:\s*(\S+)`, "version")
		t.Logf("reported version: %s", v)
		if t.Want != "" && v != t.Want {
			t.Fatalf("expected %s", t.Want)
		}
		u := t.User("1001")
		t.Logf("smoke test passed (shellcheck %s, nonroot user: %s, known-bad input flagged)", v, u)
	})
	register("policy-reporter", func(t *T) {
		out := t.Run("version")
		t.Logf("%s", out)
		if t.Want != "" {
			t.Contains(out, "AppVersion: "+t.Want, "version")
		}
		t.Contains(strings.Join(t.Files("app/templates/"), "\n"), "summary.html", "email templates")
		u := t.User("1001")
		t.Logf("smoke test passed (version, templates, nonroot user: %s)", u)
	})
	register("rabbitmq-cluster-operator", func(t *T) {
		u := t.User("1001")
		t.LabelVersion()
		t.Contains(t.RunAny("--help"), "-metrics-bind-address", "--help")
		t.Contains(t.RunAny(), "unable to find operator namespace", "startup check")
		t.Logf("smoke test passed (rabbitmq-cluster-operator %s: CLI, startup check, nonroot user %s)", t.Want, u)
	})
	register("redis-operator", func(t *T) {
		u := t.User("1001")
		t.LabelVersion()
		t.Contains(t.RunAny("--help"), "--feature-gates", "manager --help")
		// the init container runs the same image as "/operator agent bootstrap"
		t.Contains(t.RunEntryAny("/operator", "agent", "bootstrap", "--help"), "--sentinel", "agent bootstrap --help")
		t.Logf("smoke test passed (redis-operator %s: manager and agent CLI, image label, nonroot user %s)", t.Want, u)
	})
	register("tigera-operator", func(t *T) {
		t.User("1001")
		out := t.Run("-version")
		t.Logf("%s", out)
		t.WantVersionV(out)
		// its built-in image list is the Calico release the calico-* images are built from
		t.Contains(t.Run("-print-images", "list"), "calico/node:v3.32.0", "operator image list")
		t.Logf("smoke test passed")
	})
}
