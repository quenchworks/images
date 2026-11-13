package smoke

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// stampedThen is stamped plus a real subcommand that must succeed (the binary works,
// it does not just print a version).
func stampedThen(re string, version, then []string) func(*T) {
	return func(t *T) {
		out := t.Run(version...)
		t.Logf("%s", out)
		t.Match(out, "(?i)"+re, "version not stamped")
		t.Run(then...)
		u := t.User("1001")
		t.Logf("smoke test passed (nonroot user: %s)", u)
	}
}

// startupLine: an in-cluster controller logs "<what> starting version <ver>" with a stamped
// commit, then fails to reach the API server.
func startupLine(what string) func(*T) {
	return func(t *T) {
		if t.Want == "" {
			t.Fatalf("usage: qw smoke %s <image-ref> <version>", t.App)
		}
		t.User("1001")
		logs, _ := t.RunAnyFor(30*time.Second, nil)
		t.Contains(logs, what+" starting version "+t.Want, "startup line")
		t.Match(logs, `"commit":"[0-9a-f]{8}`, "commit not stamped")
		t.Logf("smoke test passed")
	}
}

func init() {
	register("jmeter", func(t *T) {
		out := t.Run("--version")
		t.Logf("%s", out)
		// the banner proves the launcher found the JRE and the jars
		t.Match(out, `[0-9]+\.[0-9]+`, "no version in output")
		t.Match(out, `(?i)apache jmeter|copyright`, "JMeter banner")
		u := t.User("1001")
		t.Logf("smoke test passed (nonroot user: %s)", u)
	})
	register("kubescape-storage", execs("/usr/bin/storage", "/usr/bin/migration", "/usr/bin/cpexport"))
	register("mariadb-operator", func(t *T) {
		t.User("1001")
		// no version stamp: check every role the chart and the MariaDB pods use
		help, err := t.RunFlags([]string{"--read-only"}, "--help")
		if err != nil {
			t.Fatalf("--help: %v %s", err, help)
		}
		for _, cmd := range []string{"webhook", "cert-controller", "init", "agent", "backup"} {
			t.Match(help, `(?m)^  `+cmd+` `, "subcommand "+cmd)
			if out, err := t.RunFlags([]string{"--read-only"}, cmd, "--help"); err != nil {
				t.Fatalf("%s --help: %v %s", cmd, err, out)
			}
		}
		t.Logf("roles: webhook cert-controller init agent backup\nsmoke test passed")
	})
	register("metallb-controller", startupLine("MetalLB controller"))
	register("metallb-speaker", startupLine("MetalLB speaker"))
	register("nsq", func(t *T) {
		for _, b := range []string{"nsqd", "nsqlookupd", "nsqadmin"} {
			out := t.RunEntry(b, "-version")
			t.Logf("%s", out)
			t.Match(out, `(?i)^`+b+` v[0-9]+\.[0-9]+\.[0-9]+`, b+" version not stamped")
		}
		u := t.User("1001")
		t.Logf("smoke test passed (nonroot user: %s)", u)
	})
	register("litestream", stampedThen(`^v?[0-9]+\.[0-9]+\.[0-9]+`, []string{"version"}, []string{"replicate", "-h"}))
	register("rclone", stampedThen(`^rclone\s+v[0-9]`, []string{"version"}, []string{"listremotes", "--help"}))
	register("syft", stampedThen(`Version:\s*[0-9]+\.[0-9]+\.[0-9]+`, []string{"version"}, []string{"scan", "--help"}))
	register("trivy", stampedThen(`Version:\s*[0-9]+\.[0-9]+`, []string{"--version"}, []string{"image", "--help"}))
	register("velero", stampedThen(`Version:\s*v?[0-9]+\.[0-9]+\.[0-9]+`, []string{"version", "--client-only"}, []string{"--help"}))
	register("velero-plugin-for-aws", func(t *T) {
		t.User("1001")
		// run it as Velero does: an init container copying the plugin into /target
		work, err := os.MkdirTemp("", "velero-plugin-")
		if err != nil {
			t.Fatalf("temp dir: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(work) })
		os.Chmod(work, 0o777)
		if out, err := t.RunFlags([]string{"--read-only", "-v", work + ":/target"}); err != nil {
			t.Fatalf("copy run: %v %s", err, out)
		}
		plugin := filepath.Join(work, "velero-plugin-for-aws")
		if fi, err := os.Stat(plugin); err != nil || fi.Mode()&0o111 == 0 {
			t.Fatalf("plugin not copied to /target as an executable")
		}
		t.Logf("  plugin copied to /target")
		out, _ := runHost(plugin)
		if !regexp.MustCompile(`This binary is a plugin`).MatchString(out) {
			t.Fatalf("copied file is not the Velero plugin: %s", out)
		}
		t.Logf("  copied binary is the Velero plugin\nsmoke test passed")
	})
}

func init() {
	register("buildkite-agent", stampedThen(`buildkite-agent version\s+[0-9]+\.[0-9]+\.[0-9]+`, []string{"--version"}, []string{"start", "--help"}))
	register("pulumi", stampedThen(`^v[0-9]+\.[0-9]+\.[0-9]+`, []string{"version"}, []string{"help"}))
	register("opentofu", func(t *T) {
		out := t.Run("version")
		t.Logf("%s", out)
		t.Match(out, `(?i)OpenTofu v[0-9]+\.[0-9]+\.[0-9]+`, "version not stamped")
		if strings.Contains(strings.ToLower(out), "dev") {
			t.Fatalf("version still reports -dev")
		}
		t.Run("init", "--help")
		u := t.User("1001")
		t.Logf("smoke test passed (nonroot user: %s)", u)
	})
	register("sealed-secrets", func(t *T) {
		ver := t.Match(t.RunAny("--version"), `controller version:\s*(\S+)`, "controller version")
		t.Logf("reported version: %s", ver)
		if ver == "0.0.0" || strings.Contains(ver, "dev") || strings.Contains(ver, "unknown") {
			t.Fatalf("version not stamped: '%s'", ver)
		}
		h := t.RunAny("--help")
		t.Contains(h, "--key-prefix", "--help")
		t.Contains(h, "--listen-addr", "--help")
		u := t.User("1001")
		t.Logf("smoke test passed (nonroot user: %s)", u)
	})
	register("external-secrets", func(t *T) {
		out := t.Run("--help")
		t.Match(out, `(?i)reconciles ExternalSecrets|external-secrets`, "--help")
		t.ContainsFold(out, "webhook", "--help subcommands")
		t.ContainsFold(out, "certcontroller", "--help subcommands")
		u := t.User("1001")
		t.Logf("smoke test passed (nonroot user: %s)", u)
	})
	register("hubble-ui", func(t *T) {
		t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp", "-p", "127.0.0.1:18081:8081"})
		const b = "http://127.0.0.1:18081"
		t.WaitHTTP(b+"/healthz", 30, name)
		_, idx := Get(b + "/")
		if !strings.Contains(idx, `<div id="app"`) && !strings.Contains(idx, "bundle.main") {
			t.Fatalf("index is not the Hubble UI")
		}
		js := regexp.MustCompile(`bundle\.main\.[0-9a-f]+\.js`).FindString(idx)
		if code, _ := Get(b + "/" + js); js == "" || code != 200 {
			t.Fatalf("main bundle not served")
		}
		// deep links go through the index fallback
		if _, body := Get(b + "/kube-system/some/deep/link"); !strings.Contains(body, "bundle.main") {
			t.Fatalf("no SPA fallback")
		}
		// no backend in this test: a gateway error proves the proxy is wired
		if code, _ := Get(b + "/api/"); code != 502 {
			t.Fatalf("/api/ gave %d, want 502 without a backend", code)
		}
		t.Logf("smoke test passed")
	})
	register("miniflux", func(t *T) {
		out := t.Run("-version")
		t.Logf("%s", out)
		// the stamped release, not the "2.3.x-dev" build-info fallback
		t.Match(out, `^[0-9]+\.[0-9]+\.[0-9]+$`, "version not stamped")
		t.Logf("build info:\n%s", t.RunAny("-info"))
		u := t.User("1001")
		t.Logf("smoke test passed (nonroot user: %s)", u)
	})
	register("cilium", func(t *T) {
		// root by design: upstream's chart runs the agent and its host-writing init containers as the image user
		t.User("0")
		sh := func(c string) string { return t.RunEntry("/bin/bash", "-c", c) }
		out := sh(`cilium-agent --version; hubble version; cilium-dbg --help >/dev/null && echo cilium-dbg ok`)
		t.Logf("%s", out)
		t.WantVersion(out)
		// the image's own clang compiles a BPF object with the shipped headers, as the agent does on a node
		out = sh(`printf "%s\n" "#include <bpf/ctx/skb.h>" "#include <bpf/api.h>" "__section(\"tc\") int t(struct __ctx_buff *ctx) { return 0; }" "BPF_LICENSE(\"Dual BSD/GPL\");" > /tmp/t.c &&
  clang -O2 --target=bpf -nostdinc -I/var/lib/cilium/bpf/include -I/var/lib/cilium/bpf -c /tmp/t.c -o /tmp/t.o && llc --version | head -2 && bpftool version | head -1 && ls -l /tmp/t.o`)
		t.Logf("%s", out)
		t.Contains(out, "t.o", "BPF object")
		out = sh(`CNI_COMMAND=VERSION /cni/loopback </dev/null; echo; ls /install-plugin.sh /init-container.sh /cni-uninstall.sh; ip -V; iptables --version; command -v ipset kmod`)
		t.Logf("%s", out)
		t.Contains(out, "cniVersion", "CNI loopback")
		t.Logf("smoke test passed")
	})
	register("jenkins-inbound-agent", func(t *T) {
		ep := t.Docker("inspect", t.Image, "--format", "{{json .Config.Entrypoint}}")
		t.Contains(ep, "jenkins-agent", "entrypoint")
		jv, _ := t.RunFlags([]string{"--entrypoint", "java"}, "-version")
		t.Match(jv, `(?i)openjdk|version`, "no working JRE")
		t.RunEntry("sh", "-c", "test -s /usr/share/jenkins/agent.jar")
		u := t.User("1001")
		t.Logf("smoke test passed (entrypoint, JRE, agent.jar, nonroot user: %s)", u)
	})
	register("k6", func(t *T) {
		out := t.Run("version")
		t.Logf("%s", out)
		t.Match(out, `(?i)k6\s+v?[0-9]+\.[0-9]+\.[0-9]+`, "version not reported")
		// k6 runs a script from stdin on a read-only root as nonroot
		if out, err := t.RunInputFlags([]string{"--read-only"}, "export default function () {};\n", "run", "--quiet", "--vus", "1", "--iterations", "1", "-"); err != nil {
			t.Fatalf("k6 run failed under read-only rootfs: %v %s", err, out)
		}
		u := t.User("1001")
		t.Logf("smoke test passed (nonroot user: %s)", u)
	})
}
