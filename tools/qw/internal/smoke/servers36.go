package smoke

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// mtlsCerts writes ca.crt plus server.{crt,key} and client.{crt,key} (CN server / client,
// both with SAN san and server+client auth) into dir.
func mtlsCerts(dir, san string) {
	w := func(name, typ string, b []byte) {
		os.WriteFile(filepath.Join(dir, name), pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: b}), 0o644)
	}
	ck, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "smoke-ca"}, NotBefore: time.Now().Add(-time.Hour),
		NotAfter: time.Now().Add(24 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	cder, _ := x509.CreateCertificate(rand.Reader, ca, ca, &ck.PublicKey, ck)
	ca, _ = x509.ParseCertificate(cder)
	w("ca.crt", "CERTIFICATE", cder)
	for i, n := range []string{"server", "client"} {
		k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		leaf := &x509.Certificate{SerialNumber: big.NewInt(int64(i + 2)), Subject: pkix.Name{CommonName: n}, DNSNames: []string{san},
			NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature,
			ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}}
		der, _ := x509.CreateCertificate(rand.Reader, leaf, ca, &k.PublicKey, ck)
		kd, _ := x509.MarshalECPrivateKey(k)
		w(n+".crt", "CERTIFICATE", der)
		w(n+".key", "EC PRIVATE KEY", kd)
	}
}

func init() {
	register("ghostunnel", func(t *T) {
		u := t.User("1001")
		if v := t.RunAny("--version"); t.Want != "" {
			t.Contains(v, t.Want, "version")
		}
		net_, srv, cli, vol := t.Name("net"), t.Name("srv"), t.Name("cli"), t.Name("certs")
		w := t.TempDir()
		mtlsCerts(w, srv)
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		cp := t.Name("cp")
		t.Docker("create", "--name", cp, "-v", vol+":/certs", t.Image)
		t.Docker("cp", w+"/.", cp+":/certs/")
		t.Docker("rm", cp)
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Cleanup(func() { docker("rm", "-f", srv, cli) })
		t.Docker("run", "-d", "--name", srv, "--network", net_, "--read-only", "-v", vol+":/certs:ro", "-p", "127.0.0.1:18443:8443", t.Image,
			"server", "--listen", "0.0.0.0:8443", "--target", "127.0.0.1:6060", "--status", "http://127.0.0.1:6060",
			"--cert", "/certs/server.crt", "--key", "/certs/server.key", "--cacert", "/certs/ca.crt", "--allow-cn", "client")
		t.Docker("run", "-d", "--name", cli, "--network", net_, "--read-only", "-v", vol+":/certs:ro", "-p", "127.0.0.1:18080:8080", t.Image,
			"client", "--listen", "0.0.0.0:8080", "--unsafe-listen", "--target", srv+":8443",
			"--cert", "/certs/client.crt", "--key", "/certs/client.key", "--cacert", "/certs/ca.crt")
		t.waitFor(30, time.Second, cli, "status not reachable through the tunnel", func() bool {
			_, b := Get("http://127.0.0.1:18080/_status")
			return strings.Contains(b, `"ok"`)
		})
		caPEM, _ := os.ReadFile(filepath.Join(w, "ca.crt"))
		pool := x509.NewCertPool()
		pool.AppendCertsFromPEM(caPEM)
		nocert := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool, ServerName: srv},
			DialContext:     (&net.Dialer{}).DialContext,
		}}
		if r, err := nocert.Get("https://127.0.0.1:18443/_status"); err == nil {
			r.Body.Close()
			t.Fatalf("server accepted a client without a certificate (HTTP %d)", r.StatusCode)
		}
		t.Logf("smoke test passed: mTLS server and client modes, certificate-less client refused, user %s", u)
	})
	register("n8n", func(t *T) {
		name := t.Name("")
		t.Start(name, []string{"--tmpfs", "/home/node:rw,mode=0777,uid=1001,gid=1001", "--tmpfs", "/tmp:rw,mode=1777",
			"-e", "N8N_DIAGNOSTICS_ENABLED=false", "-e", "N8N_VERSION_NOTIFICATIONS_ENABLED=false", "-p", "127.0.0.1:5678:5678"})
		t.waitCode("http://127.0.0.1:5678/", 90, 2*time.Second, name, 200)
		_, h := Get("http://127.0.0.1:5678/healthz")
		t.ContainsFold(h, "ok", "healthz")
		u := t.User("1001")
		// the simple-git VEX holds only while the image has no git binary
		t.RunEntry("/usr/bin/node", "-e", `
const fs = require("fs"), path = require("path"), hits = [];
(function walk(d) {
  let es; try { es = fs.readdirSync(d, { withFileTypes: true }); } catch { return; }
  for (const e of es) {
    const p = path.join(d, e.name);
    if (e.isDirectory()) { if (!["/proc", "/sys", "/dev"].includes(p)) walk(p); }
    else if (e.name === "git") hits.push(p);
  }
})("/");
if (hits.length) { console.error("git in image, the simple-git VEX no longer holds:", hits); process.exit(1); }
require("/opt/n8n/node_modules/simple-git").simpleGit("/").version().then(v => {
  if (v.installed) { console.error("simple-git found git:", JSON.stringify(v)); process.exit(1); }
  console.log("no git executable in image");
});`)
		t.Logf("smoke test passed (UI, healthz, no git for the simple-git VEX; nonroot user: %s)", u)
	})
	register("odoo", func(t *T) {
		const pgImg = "ghcr.io/quenchworks/images/postgresql@sha256:4c0580fd2968b8d6e8fb008e0eb74b55c54464b041c235842e4d1cc37ed9a445"
		v := t.RunEntry("/opt/odoo/venv/bin/python", "-c", `import importlib.metadata as m; print(m.version("odoo"))`)
		if t.Want != "" {
			// odoo publishes 19.0.20261112 as the PEP 440 version 19.0.post20261112
			i := strings.LastIndex(t.Want, ".")
			if want := t.Want[:i] + ".post" + t.Want[i+1:]; v != want {
				t.Fatalf("odoo %s, expected %s", v, want)
			}
		}
		net_, pg, app := t.Name("net"), t.Name("pg"), t.Name("")
		pw := t.Name("pg")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Cleanup(func() { docker("rm", "-f", pg) })
		t.Docker("run", "-d", "--name", pg, "--network", net_, "--network-alias", "pg", "-e", "POSTGRES_USER=odoo", "-e", "POSTGRES_PASSWORD="+pw, "-e", "POSTGRES_DB=odoodb", pgImg)
		t.waitFor(60, time.Second, pg, "postgres never ready", func() bool {
			_, err := docker("exec", pg, "pg_isready", "-h", "127.0.0.1", "-U", "odoo", "-d", "odoodb")
			return err == nil
		})
		ro := []string{"--network", net_, "--read-only", "--tmpfs", "/tmp", "--tmpfs", "/data:uid=1001,gid=1001"}
		db := []string{"--data-dir=/data", "--db_host=pg", "--db_user=odoo", "--db_password=" + pw, "-d", "odoodb"}
		out, _ := t.RunFlags(ro, append(db, "-i", "base", "--without-demo=all", "--stop-after-init")...)
		t.Contains(out, "Modules loaded", "base init")
		t.Start(app, append(ro, "-p", "127.0.0.1:18069:8069"), append(db, "--db-filter=^odoodb$")...)
		t.waitFor(60, time.Second, app, "never healthy", func() bool { _, h := Get("http://127.0.0.1:18069/web/health"); return strings.Contains(h, `"pass"`) })
		_, page := Get("http://127.0.0.1:18069/web/login")
		t.Contains(page, `name="login"`, "login page")
		for _, l := range strings.Split(t.Logs(app), "\n") {
			if regexp.MustCompile(` (ERROR|CRITICAL) `).MatchString(l) && !strings.Contains(l, "wkhtmltopdf") {
				t.Fatalf("errors in the server log: %s", l)
			}
		}
		t.Logf("smoke test passed (odoo %s, read-only rootfs, nonroot user: %s)", v, t.User("1001"))
	})
	register("uv", func(t *T) {
		if t.Want == "" {
			t.Fatalf("usage: qw smoke uv <image-ref> <pyminor>")
		}
		ro := []string{"--read-only", "--tmpfs", "/tmp"}
		py := append(append([]string{}, ro...), "--entrypoint", "/usr/bin/python3")
		v, _ := t.RunFlags(ro, "--version")
		t.Match(v, `^uv [0-9]`, "uv --version")
		if pv, _ := t.RunFlags(py, "--version"); !strings.HasPrefix(pv, "Python "+t.Want+".") {
			t.Fatalf("expected 'Python %s.*', got '%s'", t.Want, pv)
		}
		if out, err := t.RunFlags(ro, "pip", "list", "--python", "/usr/bin/python3"); err != nil {
			t.Fatalf("uv pip list failed against the bundled interpreter:\n%s", out)
		}
		out, _ := t.RunFlags(append(ro, "-e", "HOME=/tmp", "-e", "UV_CACHE_DIR=/tmp/uvcache"), "pip", "install", "--python", "/usr/bin/python3", "--target", "/tmp/t", "packaging")
		t.ContainsFold(out, "packaging", "uv pip install --target")
		if uid, _ := t.RunFlags(py, "-c", "import os; print(os.getuid())"); uid != "1001" {
			t.Fatalf("expected uid 1001, got '%s'", uid)
		}
		t.User("1001")
		t.Contains(t.Docker("inspect", t.Image, "--format", "{{json .Config.Entrypoint}}"), "/usr/bin/uv", "entrypoint")
		if _, err := t.RunFlags(py, "-c", `open("/should-fail","w")`); err == nil {
			t.Fatalf("rootfs was writable, expected read-only")
		}
		t.Logf("smoke test passed (%s, Python %s, read-only rootfs)", v, t.Want)
	})
}
