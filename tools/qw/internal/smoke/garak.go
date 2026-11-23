package smoke

import (
	"regexp"
	"strings"
)

func init() {
	// garak needs no model to prove itself: the built-in test.Blank generator answers every prompt with an empty
	// string. On a read-only rootfs with a writable home volume: --version reports the tag, the plugin registry lists
	// the stock probes, and a real run of the test.Test probe completes and writes its JSONL report into the volume.
	register("garak", func(t *T) {
		u := t.User("1001")
		vol := t.Name("home")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		flags := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/home/garak"}
		out, err := t.RunFlags(flags, "--version")
		if err != nil {
			t.Fatalf("--version: %v\n%s", err, out)
		}
		t.Contains(out, "v"+t.Want, "the garak version")
		probes, err := t.RunFlags(flags, "--list_probes")
		if err != nil {
			t.Fatalf("--list_probes: %v\n%s", err, probes)
		}
		n := len(regexp.MustCompile(`(?m)probes: \S+\.\S+`).FindAllString(probes, -1))
		if n < 50 {
			t.Fatalf("only %d probes listed:\n%s", n, probes)
		}
		run, err := t.RunFlags(flags, "--model_type", "test.Blank", "--probes", "test.Test", "--report_prefix", "smoke")
		if err != nil {
			t.Fatalf("test.Test run: %v\n%s", err, run)
		}
		t.Contains(run, "report closed", "the run's report")
		ls, _ := t.RunFlags(append(flags, "--entrypoint", "/opt/garak/venv/bin/python"), "-c",
			"import os; print(os.listdir('/home/garak/.local/share/garak/garak_runs'))")
		if !strings.Contains(ls, "smoke.report.jsonl") {
			t.Fatalf("no report in the volume:\n%s\n%s", ls, run)
		}
		t.Logf("smoke test passed (garak %s: --version, %d probes listed, test.Test run against test.Blank on a read-only rootfs, report written to the home volume; user %s)", t.Want, n, u)
	})
}
