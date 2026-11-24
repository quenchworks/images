package smoke

import "strings"

func init() {
	// On a read-only rootfs: the version is stamped, and a node run against a bundled CIS benchmark loads its
	// cfg/ definitions and prints a summary (the container is not a node, so the checks themselves fail or warn).
	register("kube-bench", func(t *T) {
		u := t.User("0")
		ro := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777"}
		out, err := t.RunFlags(ro, "version")
		if err != nil || !strings.Contains(out, t.Want) {
			t.Fatalf("kube-bench version: %v\n%s", err, out)
		}
		out, err = t.RunFlags(ro, "run", "--targets", "node", "--benchmark", "cis-1.12", "--noremediations")
		if err != nil || !strings.Contains(out, "== Summary") || !strings.Contains(out, "Worker Node Security Configuration") {
			t.Fatalf("kube-bench run: %v\n%s", err, out)
		}
		t.Logf("smoke test passed (kube-bench %s: version, cis-1.12 node run from the bundled cfg/; user %s)", t.Want, u)
	})
}
