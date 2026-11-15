package smoke

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

func init() {
	// The relay on a read-only rootfs with a password, then a real transfer through it: one
	// container sends a file with a code phrase, another receives it, both from this image and
	// both only able to meet through the relay (local discovery off). The received bytes must
	// match, and a receiver with the wrong relay password must get nothing.
	register("croc", func(t *T) {
		const pass = "smoke-relay-pass"
		const secret = "1234-quench-smoke-phrase"
		u := t.User("1001")
		t.Contains(t.RunAny("--version"), t.Want, "croc --version")
		net := t.Name("net")
		t.Docker("network", "create", net)
		t.Cleanup(func() { docker("network", "rm", net) })
		relay := t.Name("relay")
		t.Start(relay, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--network", net, "-e", "CROC_PASS=" + pass})
		t.waitFor(30, time.Second, relay, "the relay never logged that it listens", func() bool {
			if t.Docker("inspect", "-f", "{{.State.Running}}", relay) != "true" {
				t.Fatalf("relay exited\n%s", t.Logs(relay))
			}
			return strings.Contains(t.Logs(relay), "9009")
		})

		dir, err := os.MkdirTemp(envOr("TMPDIR", ""), "croc-smoke-")
		if err != nil {
			t.Fatalf("temp dir: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(dir) })
		in, out := filepath.Join(dir, "in"), filepath.Join(dir, "out")
		for _, d := range []string{in, out} {
			if err := os.MkdirAll(d, 0o777); err != nil {
				t.Fatalf("%v", err)
			}
			_ = os.Chmod(d, 0o777) // the containers write as uid 1001
		}
		payload := "quench croc smoke " + t.Name("payload") + "\n"
		if err := os.WriteFile(filepath.Join(in, "hello.txt"), []byte(payload), 0o644); err != nil {
			t.Fatalf("%v", err)
		}
		common := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--tmpfs", "/home/croc:rw,uid=1001,gid=1001", "--network", net,
			"-e", "CROC_SECRET=" + secret, "-e", "CROC_RELAY=" + relay + ":9009"}
		sender := t.Name("send")
		t.Start(sender, append(common, "-e", "CROC_PASS="+pass, "-v", in+":/in:ro"), "--yes", "send", "--no-local", "/in/hello.txt")
		time.Sleep(2 * time.Second) // the sender opens the room first

		if o, err := t.RunFlags(append(common, "-e", "CROC_PASS=wrong-pass", "-v", out+":/out"), "--yes", "--overwrite", "--out", "/out"); err == nil {
			if _, statErr := os.Stat(filepath.Join(out, "hello.txt")); statErr == nil {
				t.Fatalf("a receiver with the wrong relay password got the file\n%s", o)
			}
		}
		o, err := t.RunFlags(append(common, "-e", "CROC_PASS="+pass, "-v", out+":/out"), "--yes", "--overwrite", "--out", "/out")
		if err != nil {
			t.Fatalf("receive: %v\n%s\nsender:\n%s\nrelay:\n%s", err, o, t.Logs(sender), t.Logs(relay))
		}
		got, err := os.ReadFile(filepath.Join(out, "hello.txt"))
		if err != nil || string(got) != payload {
			t.Fatalf("received %q (%v), expected %q\n%s", got, err, payload, o)
		}
		t.Logf("smoke test passed (croc %s: read-only relay with a password, file sent and received through it intact, wrong relay password refused; user %s)", t.Want, u)
	})
}
