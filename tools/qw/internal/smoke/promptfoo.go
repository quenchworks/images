package smoke

import "strings"

func init() {
	// The CLI reports its version, then runs a real evaluation offline: the built-in echo provider returns the
	// prompt, one test asserts what it contains (must pass) and one asserts what it does not (must fail), on a
	// read-only root filesystem with $HOME on tmpfs. The run exits nonzero because of the failing test, so the
	// verdict is read from the JSON results.
	register("promptfoo", func(t *T) {
		v := t.Run("--version")
		t.Logf("promptfoo --version: %s", strings.TrimSpace(v))
		t.WantVersion(v)
		vol := t.Name("cfg")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		cfg := "description: quench smoke\nprompts:\n  - 'Say hello to {{name}}'\nproviders:\n  - echo\ntests:\n" +
			"  - vars: {name: quench}\n    assert:\n      - type: contains\n        value: hello to quench\n" +
			"  - vars: {name: works}\n    assert:\n      - type: contains\n        value: never-in-the-output\n"
		t.Docker("run", "--rm", "-v", vol+":/c", "--user", "0", busyboxImage, "-c",
			"printf '%s' \"$0\" > /c/promptfooconfig.yaml && chmod 0777 /c", cfg)
		out, _ := t.RunFlags([]string{"--read-only", "--tmpfs", "/home/promptfoo:rw,uid=1001,gid=1001", "-v", vol + ":/c", "-w", "/c"},
			"eval", "-c", "/c/promptfooconfig.yaml", "--no-cache", "--no-progress-bar", "-o", "/c/results.json")
		res := t.Docker("run", "--rm", "-v", vol+":/c", busyboxImage, "-c", "cat /c/results.json")
		t.Match(res, `"successes":\s*1`, "one passing test\n"+out)
		t.Match(res, `"failures":\s*1`, "one failing test\n"+out)
		t.Logf("smoke test passed (promptfoo %s: offline eval with the echo provider, a pass and a fail, read-only rootfs; user %s)", t.Want, t.User("1001"))
	})
}
