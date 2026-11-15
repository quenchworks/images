package smoke

import (
	"os"
	"path/filepath"
	"strings"
)

func init() {
	// yq on a read-only rootfs: builds a document from scratch, reads YAML from stdin, converts it to JSON.
	register("yq", func(t *T) {
		u := t.User("1001")
		t.Contains(t.RunAny("--version"), t.Want, "yq --version")
		t.Contains(t.RunAny("-n", `.quench.image = "yq" | .quench.ok = true`), "ok: true", "yq -n")
		out, err := t.RunInputFlags([]string{"--read-only"}, "a:\n  b: [1, 2, 3]\n", "-o=json", ".a.b | length")
		if err != nil || strings.TrimSpace(out) != "3" {
			t.Fatalf("yq on stdin: %v %q", err, out)
		}
		t.Logf("smoke test passed (yq %s: evaluates expressions, reads YAML on stdin, writes JSON; user %s)", t.Want, u)
	})

	// age on a read-only rootfs: age-keygen makes a key, age encrypts stdin to its recipient (armored), and
	// decrypts it back with the identity file; a wrong identity cannot decrypt.
	register("age", func(t *T) {
		u := t.User("1001")
		t.Contains(t.RunAny("--version"), t.Want, "age --version")
		key, err := t.RunInputFlags([]string{"--read-only", "--entrypoint", "/usr/bin/age-keygen"}, "")
		if err != nil || !strings.Contains(key, "AGE-SECRET-KEY-") {
			t.Fatalf("age-keygen: %v %q", err, key)
		}
		var pub, secret string
		for _, l := range strings.Split(key, "\n") {
			if strings.HasPrefix(l, "# public key: ") {
				pub = strings.TrimPrefix(l, "# public key: ")
			}
			if strings.HasPrefix(l, "AGE-SECRET-KEY-") {
				secret = l
			}
		}
		if pub == "" || secret == "" {
			t.Fatalf("no key pair in age-keygen output: %q", key)
		}
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "id.txt"), []byte(secret+"\n"), 0o644); err != nil {
			t.Fatalf("identity file: %v", err)
		}
		ct, err := t.RunInputFlags([]string{"--read-only"}, "quench age secret", "-a", "-r", pub)
		if err != nil || !strings.Contains(ct, "BEGIN AGE ENCRYPTED FILE") {
			t.Fatalf("encrypt: %v %q", err, ct)
		}
		pt, err := t.RunInputFlags([]string{"--read-only", "-v", dir + ":/k:ro"}, ct, "-d", "-i", "/k/id.txt")
		if err != nil || pt != "quench age secret" {
			t.Fatalf("decrypt: %v %q", err, pt)
		}
		other, _ := t.RunInputFlags([]string{"--read-only", "--entrypoint", "/usr/bin/age-keygen"}, "")
		for _, l := range strings.Split(other, "\n") {
			if strings.HasPrefix(l, "AGE-SECRET-KEY-") {
				_ = os.WriteFile(filepath.Join(dir, "other.txt"), []byte(l+"\n"), 0o644)
			}
		}
		if out, err := t.RunInputFlags([]string{"--read-only", "-v", dir + ":/k:ro"}, ct, "-d", "-i", "/k/other.txt"); err == nil {
			t.Fatalf("a wrong identity decrypted: %q", out)
		}
		t.Logf("smoke test passed (age %s: age-keygen key pair, armored encryption to the recipient, decryption with the identity, a wrong identity refused; user %s)", t.Want, u)
	})
}

// A throwaway age key pair made for this smoke test only (it protects nothing).
const sopsTestAgePub, sopsTestAgeKey = "age1gp5fxcr0akr7w8demup6n2qz5pf2z9yhhkg06dfz8645mkg6tfjsuvplgu", "AGE-SECRET-KEY-1WU5QWJQGNN0XHRVZ6TCUMURWU9CPL792VQ4DQM73RKACG50M678QUL3Y9D"

func init() {
	// sops on a read-only rootfs: encrypts a YAML document on stdin to an age recipient (values encrypted,
	// keys kept), decrypts it back with SOPS_AGE_KEY, and fails to decrypt without the key.
	register("sops", func(t *T) {
		u := t.User("1001")
		t.Contains(t.RunAny("--version", "--disable-version-check"), t.Want, "sops --version")
		plain := "db:\n  password: quench-sops-secret\n"
		enc, err := t.RunInputFlags([]string{"--read-only"}, plain, "--disable-version-check", "encrypt", "--age", sopsTestAgePub,
			"--input-type", "yaml", "--output-type", "yaml", "/dev/stdin")
		if err != nil || !strings.Contains(enc, "ENC[AES256_GCM") || strings.Contains(enc, "quench-sops-secret") {
			t.Fatalf("encrypt: %v %q", err, enc)
		}
		dec, err := t.RunInputFlags([]string{"--read-only", "-e", "SOPS_AGE_KEY=" + sopsTestAgeKey}, enc, "--disable-version-check", "decrypt",
			"--input-type", "yaml", "--output-type", "yaml", "/dev/stdin")
		if err != nil || !strings.Contains(dec, "password: quench-sops-secret") {
			t.Fatalf("decrypt: %v %q", err, dec)
		}
		if out, err := t.RunInputFlags([]string{"--read-only"}, enc, "--disable-version-check", "decrypt", "--input-type", "yaml", "--output-type", "yaml", "/dev/stdin"); err == nil {
			t.Fatalf("decrypted without the key: %q", out)
		}
		t.Logf("smoke test passed (sops %s: YAML encrypted to an age recipient and decrypted with the key, refused without it; user %s)", t.Want, u)
	})
}
