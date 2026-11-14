package smoke

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func init() {
	// nebula-cert from the image makes a CA, a lighthouse and a host certificate. A lighthouse and
	// a host (both with tun.disabled, so no NET_ADMIN) start from those, and the host must complete
	// a handshake with the lighthouse over UDP. A host signed by another CA must be refused. 1.12
	// binds a control socket in /var/run/nebula, so /run is a tmpfs as it is an emptyDir in a pod.
	register("nebula", func(t *T) {
		t.User("1001")
		v := t.RunAny("-version")
		t.Logf("%s", v)
		t.WantVersion(v)

		w := t.TempDir()
		cert := func(args ...string) {
			t.Docker(append([]string{"run", "--rm", "-v", w + ":/c", "-w", "/c", "--entrypoint", "/usr/bin/nebula-cert", t.Image}, args...)...)
		}
		cert("ca", "-name", "smoke-ca")
		cert("sign", "-name", "lh", "-ip", "192.168.100.1/24")
		cert("sign", "-name", "host", "-ip", "192.168.100.2/24")
		cert("ca", "-name", "rogue-ca", "-out-crt", "rogue-ca.crt", "-out-key", "rogue-ca.key")
		cert("sign", "-name", "rogue", "-ip", "192.168.100.3/24", "-ca-crt", "rogue-ca.crt", "-ca-key", "rogue-ca.key")

		net, lh, host, rogue := t.Name("net"), t.Name("lh"), t.Name("host"), t.Name("rogue")
		conf := func(name, ca string, isLH bool) {
			hosts := `["192.168.100.1"]`
			if isLH {
				hosts = "[]"
			}
			y := fmt.Sprintf(`pki: { ca: /c/%s, cert: /c/%s.crt, key: /c/%s.key }
listen: { host: 0.0.0.0, port: 4242 }
tun: { disabled: true }
lighthouse:
  am_lighthouse: %t
  hosts: %s
static_host_map: { "192.168.100.1": ["%s:4242"] }
firewall:
  outbound: [{ port: any, proto: any, host: any }]
  inbound: [{ port: any, proto: any, host: any }]
`, ca, name, name, isLH, hosts, lh)
			if err := os.WriteFile(filepath.Join(w, name+".yml"), []byte(y), 0o644); err != nil {
				t.Fatalf("write %s.yml: %v", name, err)
			}
		}
		conf("lh", "ca.crt", true)
		conf("host", "ca.crt", false)
		conf("rogue", "rogue-ca.crt", false)
		filepath.Walk(w, func(p string, _ os.FileInfo, _ error) error { os.Chmod(p, 0o755); return nil })

		t.Docker("network", "create", net)
		t.Cleanup(func() { docker("network", "rm", net) })
		flags := func() []string {
			return []string{"--network", net, "--read-only", "--tmpfs", "/run:uid=1001,gid=1001", "-v", w + ":/c:ro"}
		}
		t.Start(lh, flags(), "-config", "/c/lh.yml")
		time.Sleep(2 * time.Second)
		t.Start(host, flags(), "-config", "/c/host.yml")
		t.Start(rogue, flags(), "-config", "/c/rogue.yml")
		t.WaitLog(host, "Handshake message received", 30)
		time.Sleep(3 * time.Second)

		if strings.Contains(t.Logs(rogue), "Handshake message received") {
			t.Fatalf("a host from another CA was let in")
		}
		lhLog := t.Logs(lh)
		t.Match(lhLog, `(?i)could not find ca|certificate validation failed|not signed by|could not verify|untrusted`, "lighthouse rejecting the rogue certificate")
		if regexp.MustCompile(`panic|read-only file system`).MatchString(lhLog) {
			t.Fatalf("lighthouse crashed or wrote to the root:\n%s", lhLog)
		}
		t.Logf("smoke test passed: CA and certs from nebula-cert, lighthouse handshake, foreign CA refused, user 1001")
	})
}
