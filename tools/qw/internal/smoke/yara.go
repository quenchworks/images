package smoke

import (
	"os"
	"path/filepath"
	"strings"
)

func init() {
	// Read-only, as 1001, on a mounted rules + samples dir: a string rule matches the bad sample
	// only, the hash, magic, dotnet and math modules run, and yarac compiles the rules.
	register("yara", func(t *T) {
		u := t.User("1001")
		t.Contains(t.Run("--version"), t.Want, "yara --version")
		d, err := os.MkdirTemp("", "yara-smoke")
		if err != nil {
			t.Fatalf("temp dir: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(d) })
		os.Chmod(d, 0o755)
		files := map[string]string{
			"rules.yar": `import "hash"
import "magic"
import "dotnet"
import "math"
rule quench_string { strings: $a = "QUENCH-EVIL-MARKER" condition: $a }
rule quench_modules { condition: hash.md5(0, filesize) != "" and math.entropy(0, filesize) >= 0 and magic.type() != "" }
`,
			"samples/bad.bin":  "header QUENCH-EVIL-MARKER trailer\n",
			"samples/good.bin": "nothing to see here\n",
		}
		for name, body := range files {
			p := filepath.Join(d, name)
			os.MkdirAll(filepath.Dir(p), 0o755)
			if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
				t.Fatalf("write %s: %v", name, err)
			}
		}
		flags := []string{"--read-only", "-v", d + ":/scan:ro"}
		out, err := t.RunFlags(flags, "-r", "/scan/rules.yar", "/scan/samples")
		if err != nil {
			t.Fatalf("yara scan: %v\n%s", err, out)
		}
		t.Contains(out, "quench_string /scan/samples/bad.bin", "string rule on the bad sample")
		if strings.Contains(out, "quench_string /scan/samples/good.bin") {
			t.Fatalf("the string rule matched the clean sample:\n%s", out)
		}
		// the module rule matches both samples: hash, math and magic ran; importing dotnet proves it loads (its fields
		// are undefined on a non-PE file, which would make the rule false)
		t.Contains(out, "quench_modules /scan/samples/good.bin", "module rule")
		out, err = RunFlagsEntry(t, append(flags, "--tmpfs", "/tmp:rw,mode=1777"), "/usr/bin/yarac", "/scan/rules.yar", "/tmp/r.yarc")
		if err != nil {
			t.Fatalf("yarac: %v\n%s", err, out)
		}
		t.Logf("smoke test passed (yara %s: read-only scan, string and module rules, clean sample not matched, yarac compiles; user %s)", t.Want, u)
	})
}

// RunFlagsEntry runs the image once with docker flags and another entrypoint.
func RunFlagsEntry(t *T, flags []string, entry string, args ...string) (string, error) {
	return docker(append(append(append([]string{"run", "--rm", "--entrypoint", entry}, flags...), t.Image), args...)...)
}
