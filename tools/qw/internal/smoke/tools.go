package smoke

import (
	"os"
	"path/filepath"
	"regexp"
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
