package smoke

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// leakHistory commits dir's config.ini and then deletes it, so only git history holds it.
// It runs the image's own git (scanner images ship one), so the host needs no git.
func (t *T) leakHistory(dir string) {
	git := []string{"-v", dir + ":/repo", "-w", "/repo", "--tmpfs", "/tmp", "-e", "HOME=/tmp", "-e", "GIT_AUTHOR_NAME=t", "-e", "GIT_AUTHOR_EMAIL=t@t",
		"-e", "GIT_COMMITTER_NAME=t", "-e", "GIT_COMMITTER_EMAIL=t@t", "--entrypoint", "/usr/bin/git"}
	for _, c := range [][]string{{"init", "-q"}, {"add", "config.ini"}, {"commit", "-qm", "leak"}, {"rm", "-q", "config.ini"}, {"commit", "-qm", "clean"}, {"config", "core.sharedRepository", "all"}} {
		if out, err := t.RunFlags(git, c...); err != nil {
			t.Fatalf("git %v: %s", c, out)
		}
	}
}

// caAndServer writes caCert.pem, caKey.pem, serverCert.pem and serverKey.pem into dir.
func caAndServer(dir, cn string) {
	pemOut := func(name, typ string, b []byte) {
		os.WriteFile(filepath.Join(dir, name), pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: b}), 0o644)
	}
	ck, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "smoke-ca"}, NotBefore: time.Now().Add(-time.Hour),
		NotAfter: time.Now().Add(24 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	cder, _ := x509.CreateCertificate(rand.Reader, ca, ca, &ck.PublicKey, ck)
	ca, _ = x509.ParseCertificate(cder)
	sk, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	srv := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: cn}, DNSNames: []string{cn}, NotBefore: time.Now().Add(-time.Hour),
		NotAfter: time.Now().Add(24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	sder, _ := x509.CreateCertificate(rand.Reader, srv, ca, &sk.PublicKey, ck)
	ckd, _ := x509.MarshalECPrivateKey(ck)
	skd, _ := x509.MarshalECPrivateKey(sk)
	pemOut("caCert.pem", "CERTIFICATE", cder)
	pemOut("caKey.pem", "EC PRIVATE KEY", ckd)
	pemOut("serverCert.pem", "CERTIFICATE", sder)
	pemOut("serverKey.pem", "EC PRIVATE KEY", skd)
}

func init() {
	register("trufflehog", func(t *T) {
		u := t.User("1001")
		if v := t.Run("--version"); t.Want != "" && v != "trufflehog "+t.Want {
			t.Fatalf("version %s, expected %s", v, t.Want)
		}
		w := t.TempDir()
		leaky, clean := filepath.Join(w, "leaky"), filepath.Join(w, "clean")
		for _, d := range []string{leaky, clean} {
			os.Mkdir(d, 0o777)
			os.Chmod(d, 0o777)
		}
		os.WriteFile(filepath.Join(leaky, "config.ini"), []byte("token = ghp_"+"Xk9Lm2Qp7Rt4Vw8Yz1Bc3Df5Gh6Jn0Ps2Tu4\n"), 0o666)
		os.WriteFile(filepath.Join(clean, "config.ini"), []byte("hello = \"world\"\n"), 0o644)
		scan := func(dir string, args ...string) (string, int) {
			return t.RunCode([]string{"--read-only", "--tmpfs", "/tmp", "-v", dir + ":/scan:ro"}, append(args, "--no-verification", "--json", "--fail")...)
		}
		out, code := scan(leaky, "filesystem", "/scan")
		if code != 183 || !strings.Contains(out, `"DetectorName":"Github"`) {
			t.Fatalf("planted token not reported (exit %d): %s", code, out)
		}
		if out, code := scan(clean, "filesystem", "/scan"); code != 0 {
			t.Fatalf("clean directory reported (exit %d): %s", code, out)
		}
		t.leakHistory(leaky)
		out, code = scan(leaky, "git", "file:///scan")
		if code != 183 || !strings.Contains(out, `"DetectorName":"Github"`) {
			t.Fatalf("token in history not reported (exit %d): %s", code, out)
		}
		t.Logf("smoke test passed (filesystem and git history scans, uid %s)", u)
	})
	register("xwiki", func(t *T) {
		const pgImg = "ghcr.io/quenchworks/images/postgresql@sha256:4c0580fd2968b8d6e8fb008e0eb74b55c54464b041c235842e4d1cc37ed9a445"
		u := t.User("1001")
		net_, pg, app := t.Name("net"), t.Name("pg"), t.Name("")
		pw := t.Name("pw")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Cleanup(func() { docker("rm", "-f", pg) })
		t.Docker("run", "-d", "--name", pg, "--network", net_, "--network-alias", "pg", "-e", "POSTGRES_PASSWORD="+pw, "-e", "POSTGRES_DB=xwiki", pgImg)
		t.waitFor(60, time.Second, pg, "postgres never ready", func() bool {
			_, err := docker("exec", pg, "pg_isready", "-h", "127.0.0.1", "-U", "postgres", "-d", "xwiki")
			return err == nil
		})
		t.Start(app, []string{"--network", net_, "--read-only", "--tmpfs", "/tmp", "--tmpfs", "/var/lib/xwiki:uid=1001,gid=1001", "-p", "127.0.0.1:18080:8080",
			"-e", "DB_HOST=pg", "-e", "DB_DATABASE=xwiki", "-e", "DB_USER=postgres", "-e", "DB_PASSWORD=" + pw})
		var body string
		t.waitFor(120, 3*time.Second, app, "REST root never answered 200", func() bool {
			c, b := Get("http://127.0.0.1:18080/xwiki/rest")
			body = b
			return c == 200
		})
		m := regexp.MustCompile(`<version>([^<]*)</version>`).FindStringSubmatch(body)
		if m == nil || (t.Want != "" && m[1] != t.Want) {
			t.Fatalf("version %v, expected %s", m, t.Want)
		}
		if n, _ := docker("exec", pg, "psql", "-U", "postgres", "-d", "xwiki", "-tAc", "select count(*) from information_schema.tables where table_name='xwikidoc'"); n != "1" {
			t.Fatalf("xwikidoc table missing in PostgreSQL\n%s", t.Logs(app))
		}
		t.Logf("smoke test passed (version %s, nonroot user: %s, PostgreSQL store, REST 200)", m[1], u)
	})
	register("alertmanager", func(t *T) {
		const b = "http://127.0.0.1:9093"
		name, _ := t.httpUp([]string{"--read-only", "--tmpfs", "/alertmanager:rw,mode=1777", "--tmpfs", "/tmp:rw,mode=1777", "-p", "127.0.0.1:9093:9093"}, b+"/-/healthy", 30)
		t.WaitHTTP(b+"/-/ready", 30, name)
		_, st := Get(b + "/api/v2/status")
		t.Contains(st, `"versionInfo"`, "/api/v2/status")
		_, ui := Get(b + "/")
		t.Match(ui, `(?i)<!doctype html|<html`, "embedded UI")
		t.Docker("exec", name, "/usr/bin/amtool", "--version")
		t.Logf("smoke test passed (nonroot uid: %s, read-only rootfs, healthy+ready+status+UI)", t.User("1001"))
	})
	register("nginx", func(t *T) {
		const b = "http://127.0.0.1:8080"
		name, body := t.httpUp([]string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-p", "127.0.0.1:8080:8080"}, b+"/", 30)
		t.ContainsFold(body, "nginx", "default page")
		_, st := Get(b + "/stub_status")
		t.ContainsFold(st, "Active connections", "stub_status")
		if regexp.MustCompile(`(?i)read-only file system|permission denied`).MatchString(t.Logs(name)) {
			t.Fatalf("read-only/permission errors in logs\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed (nonroot user: %s)", t.User("1001"))
	})
	register("proxysql", func(t *T) {
		client := envOr("MYSQL_CLIENT_IMAGE", "ghcr.io/quenchworks/images/mariadb:11.8.9")
		t.Docker("pull", "-q", client) // else the first query returns pull progress
		v := regexp.MustCompile(`ProxySQL version [0-9]+\.[0-9]+\.[0-9]+\S*`).FindString(t.RunEntryAny("/usr/bin/proxysql", "--version"))
		if v == "" || (t.Want != "" && !strings.Contains(v, "version "+t.Want)) {
			t.Fatalf("version %q, expected %s", v, t.Want)
		}
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/var/lib/proxysql:rw,uid=1001,gid=1001", "--tmpfs", "/tmp:rw,mode=1777"})
		sql := func(port, user, pw, q string) (string, error) {
			return docker("run", "--rm", "--network", "container:"+name, "--entrypoint", "/usr/bin/mariadb", client,
				"-h", "127.0.0.1", "-P", port, "-u", user, "-p"+pw, "--skip-ssl", "-N", "-B", "-e", q)
		}
		var av string
		t.waitFor(45, time.Second, name, "admin interface never answered", func() bool {
			if s := t.Docker("inspect", "-f", "{{.State.Status}}", name); s != "running" {
				t.Fatalf("container died during startup\n%s", t.Logs(name))
			}
			out, err := sql("6032", "admin", "admin", `SELECT variable_value FROM global_variables WHERE variable_name="admin-version"`)
			av = out
			return err == nil && out != ""
		})
		t.Match(av, `^[0-9]+\.[0-9]+\.[0-9]+`, "admin-version")
		rt, _ := sql("6032", "admin", "admin", "INSERT INTO mysql_servers (hostgroup_id, hostname, port) VALUES (0, 'db.quench.invalid', 3306); LOAD MYSQL SERVERS TO RUNTIME; SELECT hostname FROM runtime_mysql_servers")
		t.Match(rt, `(?m)^db\.quench\.invalid$`, "server loaded to runtime")
		e, _ := sql("6033", "nobody", "wrong", "SELECT 1")
		t.Contains(e, "ERROR 1045", "client port refuses an unknown user")
		if m := regexp.MustCompile(`Read-only file system|[Pp]ermission denied|unable to open database`).FindString(t.Logs(name)); m != "" {
			t.Fatalf("errors in logs (%s)\n%s", m, t.Logs(name))
		}
		t.Logf("smoke test passed (nonroot %s, read-only rootfs, admin SQL and client handshake)", t.User("1001"))
	})
	register("vertical-pod-autoscaler", func(t *T) {
		u := t.User("1001")
		w := t.TempDir()
		b, _ := os.ReadFile(t.DeadKubeconfig())
		os.WriteFile(filepath.Join(w, "kc"), b, 0o644)
		caAndServer(w, "vpa-webhook")
		run := func(c, hp, cp string, args ...string) {
			n := t.Name(c)
			t.Docker(append([]string{"create", "--name", n, "-p", "127.0.0.1:" + hp + ":" + cp, "--entrypoint", "/usr/bin/vpa-" + c, t.Image, "--kubeconfig=/work/kc"}, args...)...)
			t.Cleanup(func() { docker("rm", "-f", n) })
			t.Docker("cp", w+"/.", n+":/work")
			t.Docker("start", n)
			t.waitFor(30, time.Second, n, c+" /metrics never answered", func() bool {
				if s := t.Docker("inspect", "-f", "{{.State.Status}}", n); s != "running" {
					t.Fatalf("%s exited\n%s", c, t.Logs(n))
				}
				code, _ := Get("http://127.0.0.1:" + hp + "/metrics")
				return code == 200
			})
		}
		run("recommender", "18942", "8942")
		run("updater", "18943", "8943")
		run("admission-controller", "18944", "8944", "--register-webhook=false", "--client-ca-file=/work/caCert.pem",
			"--tls-cert-file=/work/serverCert.pem", "--tls-private-key=/work/serverKey.pem")
		t.Logf("smoke test passed (uid %s, three components up against a dead API server)", u)
	})
}
