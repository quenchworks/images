package smoke

import (
	"os"
	"path/filepath"
	"strings"
)

func init() {
	// kubeconform on a read-only rootfs, offline: a custom kind validated against a local JSON schema;
	// a valid document passes and one with a wrong type fails.
	register("kubeconform", func(t *T) {
		u := t.User("1001")
		t.Contains(t.RunAny("-v"), t.Want, "kubeconform -v")
		dir := t.TempDir()
		schema := `{"type":"object","required":["spec"],"properties":{"spec":{"type":"object","required":["replicas"],"properties":{"replicas":{"type":"integer"}}}}}`
		files := map[string]string{
			"quench.json": schema, // kubeconform lowercases .ResourceKind
			"good.yaml":   "apiVersion: quench/v1\nkind: Quench\nmetadata:\n  name: ok\nspec:\n  replicas: 2\n",
			"bad.yaml":    "apiVersion: quench/v1\nkind: Quench\nmetadata:\n  name: bad\nspec:\n  replicas: two\n",
		}
		for n, c := range files {
			if err := os.WriteFile(filepath.Join(dir, n), []byte(c), 0o644); err != nil {
				t.Fatalf("write %s: %v", n, err)
			}
		}
		flags := []string{"--read-only", "-v", dir + ":/w:ro"}
		loc := "/w/{{ .ResourceKind }}.json"
		if out, err := t.RunInputFlags(flags, "", "-strict", "-summary", "-schema-location", loc, "/w/good.yaml"); err != nil {
			t.Fatalf("a valid document failed: %v\n%s", err, out)
		}
		if out, err := t.RunInputFlags(flags, "", "-strict", "-schema-location", loc, "/w/bad.yaml"); err == nil || !strings.Contains(out, "replicas") {
			t.Fatalf("an invalid document passed: %v\n%s", err, out)
		}
		t.Logf("smoke test passed (kubeconform %s: offline validation against a local schema, valid passes, invalid fails; user %s)", t.Want, u)
	})

	// kube-linter on a read-only rootfs: a bare Deployment gets findings, a hardened one passes the
	// run-as-non-root and read-only-root-fs checks.
	register("kube-linter", func(t *T) {
		u := t.User("1001")
		t.Contains(t.RunAny("version"), t.Want, "kube-linter version")
		dir := t.TempDir()
		bad := "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: bad\nspec:\n  selector:\n    matchLabels: {app: bad}\n  template:\n    metadata:\n      labels: {app: bad}\n    spec:\n      containers:\n        - name: app\n          image: example/app:latest\n"
		good := "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: good\nspec:\n  selector:\n    matchLabels: {app: good}\n  template:\n    metadata:\n      labels: {app: good}\n    spec:\n      securityContext:\n        runAsNonRoot: true\n      containers:\n        - name: app\n          image: example/app@sha256:0000000000000000000000000000000000000000000000000000000000000000\n          securityContext:\n            readOnlyRootFilesystem: true\n            runAsNonRoot: true\n"
		_ = os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte(bad), 0o644)
		_ = os.WriteFile(filepath.Join(dir, "good.yaml"), []byte(good), 0o644)
		flags := []string{"--read-only", "-v", dir + ":/w:ro"}
		out, err := t.RunInputFlags(flags, "", "lint", "/w/bad.yaml")
		if err == nil || !strings.Contains(out, "run-as-non-root") {
			t.Fatalf("the bare Deployment got no run-as-non-root finding: %v\n%s", err, out)
		}
		if out, err := t.RunInputFlags(flags, "", "lint", "--do-not-auto-add-defaults", "--include", "run-as-non-root,no-read-only-root-fs", "/w/good.yaml"); err != nil {
			t.Fatalf("the hardened Deployment failed: %v\n%s", err, out)
		}
		t.Logf("smoke test passed (kube-linter %s: findings on a bare Deployment, a hardened one passes; user %s)", t.Want, u)
	})

	// stern needs a cluster for real work: version, and without a kubeconfig it fails with a kubeconfig error
	// instead of crashing.
	register("stern", func(t *T) {
		u := t.User("1001")
		t.Contains(t.RunAny("--version"), t.Want, "stern --version")
		out, err := t.RunInputFlags([]string{"--read-only"}, "", "--kubeconfig", "/nonexistent", ".")
		if err == nil || strings.Contains(out, "panic") {
			t.Fatalf("stern without a cluster: %v\n%s", err, out)
		}
		t.Logf("smoke test passed (stern %s: version; without a cluster it exits with an error, no panic; user %s)", t.Want, u)
	})

	// k9s is a terminal UI that needs a cluster: version and info run on a read-only rootfs with a writable /tmp.
	register("k9s", func(t *T) {
		u := t.User("1001")
		t.Contains(t.RunAny("version", "-s"), t.Want, "k9s version")
		out, err := t.RunInputFlags([]string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-e", "XDG_CONFIG_HOME=/tmp", "-e", "XDG_STATE_HOME=/tmp", "-e", "XDG_DATA_HOME=/tmp"}, "", "info")
		if err != nil || !strings.Contains(out, "Context Configs") {
			t.Fatalf("k9s info: %v\n%s", err, out)
		}
		t.Logf("smoke test passed (k9s %s: version and info on a read-only rootfs; user %s)", t.Want, u)
	})
}

func init() {
	// helm on a read-only rootfs, offline: lints and renders a small local chart with a --set value.
	register("helm", func(t *T) {
		u := t.User("1001")
		t.Contains(t.RunAny("version", "--short"), t.Want, "helm version")
		dir := t.TempDir()
		_ = os.MkdirAll(filepath.Join(dir, "quench", "templates"), 0o777)
		files := map[string]string{
			"quench/Chart.yaml":            "apiVersion: v2\nname: quench\nversion: 0.1.0\n",
			"quench/values.yaml":           "greeting: hello\n",
			"quench/templates/config.yaml": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: {{ .Release.Name }}-quench\ndata:\n  greeting: {{ .Values.greeting | quote }}\n",
		}
		for n, c := range files {
			if err := os.WriteFile(filepath.Join(dir, n), []byte(c), 0o644); err != nil {
				t.Fatalf("write %s: %v", n, err)
			}
		}
		flags := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-e", "HELM_CACHE_HOME=/tmp/c", "-e", "HELM_CONFIG_HOME=/tmp/g", "-e", "HELM_DATA_HOME=/tmp/d", "-v", dir + ":/w:ro"}
		if out, err := t.RunInputFlags(flags, "", "lint", "/w/quench"); err != nil {
			t.Fatalf("helm lint: %v\n%s", err, out)
		}
		out, err := t.RunInputFlags(flags, "", "template", "smoke", "/w/quench", "--set", "greeting=quench-helm")
		if err != nil || !strings.Contains(out, `greeting: "quench-helm"`) || !strings.Contains(out, "name: smoke-quench") {
			t.Fatalf("helm template: %v\n%s", err, out)
		}
		t.Logf("smoke test passed (helm %s: lint and template of a local chart with --set, offline, read-only; user %s)", t.Want, u)
	})

	// popeye scans a live cluster: version, and without a kubeconfig it exits with an error instead of a panic.
	register("popeye", func(t *T) {
		u := t.User("1001")
		t.Contains(t.RunAny("version"), t.Want, "popeye version")
		out, err := t.RunInputFlags([]string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-e", "HOME=/tmp"}, "", "--kubeconfig", "/nonexistent")
		if err == nil || strings.Contains(out, "panic") {
			t.Fatalf("popeye without a cluster: %v\n%s", err, out)
		}
		t.Logf("smoke test passed (popeye %s: version; without a cluster it exits with an error, no panic; user %s)", t.Want, u)
	})
}
