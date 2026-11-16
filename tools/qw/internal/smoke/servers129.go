package smoke

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"time"
)

// pebbleImage is Let's Encrypt's ACME test CA, used only as the smoke test's counterpart.
const pebbleImage = "ghcr.io/letsencrypt/pebble@sha256:0c81f05db371f436c97fb894bd6d2ab8fc9c8e2ce40f8bef8a8e15941bcd75ef"

func init() {
	// On a read-only rootfs: lego registers an account with a local ACME CA (Pebble), answers a real HTTP-01
	// challenge that Pebble's validator fetches over the network, and stores the issued certificate.
	register("lego", func(t *T) {
		const domain = "legoclient.quench.test"
		u := t.User("1001")
		t.Contains(t.RunAny("--version"), t.Want, "lego --version")
		net := t.Name("net")
		t.Docker("network", "create", net)
		t.Cleanup(func() { docker("network", "rm", net) })
		pb := t.Name("pebble")
		t.Docker("run", "-d", "--name", pb, "--network", net, "--network-alias", "pebble", pebbleImage, "-config", "test/config/pebble-config.json")
		t.Cleanup(func() { docker("rm", "-f", pb) })
		dir := t.TempDir()
		if out, err := docker("cp", pb+":/test/certs/pebble.minica.pem", filepath.Join(dir, "ca.pem")); err != nil {
			t.Fatalf("copy the Pebble root: %v\n%s", err, out)
		}
		_ = os.Chmod(filepath.Join(dir, "ca.pem"), 0o644)
		time.Sleep(2 * time.Second)
		flags := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--network", net, "--network-alias", domain,
			"-v", dir + ":/w", "-e", "LEGO_CA_CERTIFICATES=/w/ca.pem"}
		out, rc := t.RunAnyFor(3*time.Minute, flags, "run", "-m", "smoke@quench-works.com", "--accept-tos",
			"-s", "https://pebble:14000/dir", "-d", domain, "--http", "--http.address", ":5002", "--path", "/w/lego")
		if rc != 0 {
			t.Fatalf("lego run failed (%d):\n%s\n%s", rc, out, t.Logs(pb))
		}
		crts, _ := filepath.Glob(filepath.Join(dir, "lego", "*", domain+".crt"))
		if len(crts) == 0 {
			t.Fatalf("no certificate for %s under --path:\n%s", domain, out)
		}
		b, _ := os.ReadFile(crts[0])
		blk, _ := pem.Decode(b)
		if blk == nil {
			t.Fatalf("%s is not PEM", crts[0])
		}
		c, err := x509.ParseCertificate(blk.Bytes)
		if err != nil || len(c.DNSNames) != 1 || c.DNSNames[0] != domain {
			t.Fatalf("issued certificate: %v %v", err, c)
		}
		t.Logf("smoke test passed (lego %s: account registered and a certificate for %s issued by a local ACME CA through a real HTTP-01 challenge, read-only rootfs; user %s)", t.Want, domain, u)
	})
}
