package smoke

import (
	"os"
	"path/filepath"
	"strings"
)

// writeTree writes files (relative path -> content) under dir, creating parent directories.
func writeTree(t *T, dir string, files map[string]string) {
	for n, c := range files {
		p := filepath.Join(dir, n)
		_ = os.MkdirAll(filepath.Dir(p), 0o777)
		if err := os.WriteFile(p, []byte(c), 0o666); err != nil {
			t.Fatalf("write %s: %v", n, err)
		}
	}
}

func init() {
	// kustomize builds an overlay (name prefix, label, replicas patch) on a base, offline, read-only.
	register("kustomize", func(t *T) {
		u := t.User("1001")
		t.Contains(t.RunAny("version"), t.Want, "kustomize version")
		dir := t.TempDir()
		writeTree(t, dir, map[string]string{
			"base/kustomization.yaml": "resources:\n- deploy.yaml\n",
			"base/deploy.yaml":        "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\nspec:\n  replicas: 1\n  selector:\n    matchLabels: {app: web}\n  template:\n    metadata:\n      labels: {app: web}\n    spec:\n      containers:\n      - name: web\n        image: nginx\n",
			"prod/kustomization.yaml": "resources:\n- ../base\nnamePrefix: prod-\nlabels:\n- pairs: {env: prod}\npatches:\n- patch: |-\n    - op: replace\n      path: /spec/replicas\n      value: 3\n  target: {kind: Deployment, name: web}\n",
		})
		out, err := t.RunInputFlags([]string{"--read-only", "-v", dir + ":/w:ro"}, "", "build", "/w/prod")
		if err != nil || !strings.Contains(out, "name: prod-web") || !strings.Contains(out, "replicas: 3") || !strings.Contains(out, "env: prod") {
			t.Fatalf("kustomize build: %v\n%s", err, out)
		}
		t.Logf("smoke test passed (kustomize %s: overlay with prefix, label and patch built offline, read-only; user %s)", t.Want, u)
	})

	// tflint lints a module with its bundled terraform ruleset and reports a planted unused variable.
	register("tflint", func(t *T) {
		u := t.User("1001")
		t.Contains(t.RunAny("--version"), t.Want, "tflint version")
		dir := t.TempDir()
		writeTree(t, dir, map[string]string{
			"main.tf": "terraform {\n  required_version = \">= 1.5\"\n}\n\nvariable \"planted_unused\" {\n  type = string\n}\n\noutput \"greeting\" {\n  value = \"hello\"\n}\n",
		})
		out, err := t.RunInputFlags([]string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", dir + ":/w:ro"}, "", "--chdir", "/w", "--format", "compact")
		if err == nil || !strings.Contains(out, "terraform_unused_declarations") || !strings.Contains(out, "planted_unused") {
			t.Fatalf("tflint did not report the planted issue: %v\n%s", err, out)
		}
		t.Logf("smoke test passed (tflint %s: bundled terraform ruleset reported the planted unused variable, read-only; user %s)", t.Want, u)
	})

	// task runs a Taskfile target whose dependency runs first, with built-in shell commands only.
	register("task", func(t *T) {
		u := t.User("1001")
		t.Contains(t.RunAny("--version"), t.Want, "task version")
		dir := t.TempDir()
		writeTree(t, dir, map[string]string{
			"Taskfile.yml": "version: '3'\nvars:\n  WHO: quench\ntasks:\n  gen:\n    cmds:\n    - echo generated > out.txt\n  build:\n    deps: [gen]\n    cmds:\n    - echo \"built by {{.WHO}}\" >> out.txt\n",
		})
		if out, err := t.RunInputFlags([]string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", dir + ":/w"}, "", "--dir", "/w", "build"); err != nil {
			t.Fatalf("task build: %v\n%s", err, out)
		}
		got, err := os.ReadFile(filepath.Join(dir, "out.txt"))
		if err != nil || string(got) != "generated\nbuilt by quench\n" {
			t.Fatalf("task output file: %q %v", got, err)
		}
		t.Logf("smoke test passed (task %s: dependency ran before the target, template var expanded, built-in shell, read-only rootfs; user %s)", t.Want, u)
	})
}
