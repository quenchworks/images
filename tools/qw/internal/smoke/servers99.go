package smoke

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func init() {
	// On a read-only rootfs with a config holding a bcrypt service account and a user: our
	// OpenLDAP image's ldapsearch (a real LDAP client) binds as the service account and finds the
	// user with its mail attribute, and a wrong password is refused with invalid credentials.
	register("glauth", func(t *T) {
		const pw = "Smoke-Pass-2026"
		u := t.User("1001")
		t.Contains(t.RunAny("--version"), t.Want, "glauth --version")
		hash, _ := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
		hexHash := hex.EncodeToString(hash) // GLAuth reads passbcrypt as hex
		cfg := `[ldap]
  enabled = true
  listen = "0.0.0.0:3893"
[ldaps]
  enabled = false
[backend]
  datastore = "config"
  baseDN = "dc=quench,dc=test"
[[users]]
  name = "svc"
  uidnumber = 5000
  primarygroup = 5500
  passbcrypt = "` + hexHash + `"
  [[users.capabilities]]
    action = "search"
    object = "*"
[[users]]
  name = "alice"
  mail = "alice@quench.test"
  uidnumber = 5001
  primarygroup = 5500
  passbcrypt = "` + hexHash + `"
[[groups]]
  name = "people"
  gidnumber = 5500
`
		dir, err := os.MkdirTemp(envOr("TMPDIR", ""), "glauth-smoke-")
		if err != nil {
			t.Fatalf("temp dir: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(dir) })
		if err := os.WriteFile(filepath.Join(dir, "glauth.cfg"), []byte(cfg), 0o644); err != nil {
			t.Fatalf("%v", err)
		}
		net := t.Name("net")
		t.Docker("network", "create", net)
		t.Cleanup(func() { docker("network", "rm", net) })
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--network", net,
			"-v", filepath.Join(dir, "glauth.cfg") + ":/etc/glauth/glauth.cfg:ro"})

		client := envOr("OPENLDAP_IMAGE", "ghcr.io/quenchworks/images/openldap@sha256:4ffafc0a21ea2e3ecade0ac1de3b664530da1d05efab2819b5df4db0a1139e00")
		search := func(bindPW string) (string, error) {
			return docker("run", "--rm", "--network", net, "--entrypoint", "ldapsearch", client,
				"-x", "-H", "ldap://"+name+":3893", "-D", "cn=svc,ou=people,dc=quench,dc=test", "-w", bindPW,
				"-b", "dc=quench,dc=test", "(uid=alice)", "mail")
		}
		var out string
		t.waitFor(30, time.Second, name, "glauth never answered a bind", func() bool {
			if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
				t.Fatalf("glauth exited\n%s", t.Logs(name))
			}
			o, err := search(pw)
			out = o
			return err == nil
		})
		t.Contains(out, "alice@quench.test", "alice's mail found by the service account")
		if o, err := search("wrong-password-1"); err == nil || !strings.Contains(o, "Invalid credentials") {
			t.Fatalf("a wrong password was not refused as invalid credentials: %v\n%s", err, o)
		}
		t.Logf("smoke test passed (glauth %s: read-only boot, bcrypt service account binds and finds a user with ldapsearch, wrong password refused; user %s)", t.Want, u)
	})
}
