package smoke

import (
	"net/http"
	"strings"
	"time"
)

func init() {
	// Against the catalog's PostgreSQL, on a read-only root filesystem: WildFly deploys ejbca.ear, the health check
	// answers ALLOK, the EJB CLI creates a CA and issues a PEM keystore for an end entity, the CA's CRL downloads
	// over HTTP, and the anonymous CLI listener (4447) listens on loopback only.
	register("ejbca", func(t *T) {
		const base = "http://127.0.0.1:18120"
		const dbpw = "Smoke-Db-2026"
		u := t.User("1001")
		net := t.Name("net")
		t.Docker("network", "create", net)
		t.Cleanup(func() { docker("network", "rm", net) })
		db := t.Name("db")
		t.Docker("run", "-d", "--name", db, "--network", net, "--network-alias", "db", "-e", "POSTGRES_PASSWORD="+dbpw,
			"-e", "POSTGRES_USER=ejbca", "-e", "POSTGRES_DB=ejbca_db", leantimePostgres)
		t.Cleanup(func() { docker("rm", "-f", db) })
		t.waitFor(60, 2*time.Second, db, "postgres did not become ready", func() bool {
			_, err := docker("exec", "-e", "PGPASSWORD="+dbpw, db, "psql", "-h", "127.0.0.1", "-U", "ejbca", "-d", "ejbca_db", "-tAc", "select 1")
			return err == nil
		})
		name := t.Name("")
		t.Start(name, []string{"--network", net, "--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-p", "127.0.0.1:18120:8080",
			"-e", "DATABASE_JDBC_URL=jdbc:postgresql://db:5432/ejbca_db", "-e", "DATABASE_USER=ejbca", "-e", "DATABASE_PASSWORD=" + dbpw})
		t.waitFor(150, 2*time.Second, name, "ejbcahealth did not answer ALLOK", func() bool {
			c, b := Get(base + "/ejbca/publicweb/healthcheck/ejbcahealth")
			return c == http.StatusOK && strings.Contains(b, "ALLOK")
		})
		cli := func(args ...string) string {
			out, err := docker(append([]string{"exec", name, "ejbca"}, args...)...)
			if err != nil {
				t.Fatalf("ejbca %s: %v\n%s\n%s", strings.Join(args, " "), err, out, t.Logs(name))
			}
			return out
		}
		cli("ca", "init", "--caname", "SmokeCA", "--dn", "CN=SmokeCA,O=Quenchworks", "--tokenType", "soft", "--tokenPass", "null",
			"--keytype", "RSA", "--keyspec", "2048", "-v", "365", "--policy", "null", "-s", "SHA256WithRSA")
		t.Contains(cli("ca", "listcas"), "CN=SmokeCA,O=Quenchworks", "ca listcas")
		cli("ra", "addendentity", "--username", "smoke", "--dn", "CN=smoke.example", "--caname", "SmokeCA", "--type", "1",
			"--token", "PEM", "--password", "Smoke-Entity-2026")
		cli("ra", "setclearpwd", "smoke", "Smoke-Entity-2026")
		cli("batch", "smoke", "-dir", "/tmp/out")
		pem, err := docker("exec", name, "sh", "-c", "cat /tmp/out/pem/*.pem")
		if err != nil || !strings.Contains(pem, "BEGIN CERTIFICATE") || !strings.Contains(pem, "PRIVATE KEY") {
			t.Fatalf("no issued PEM keystore: %v\n%s", err, pem)
		}
		c, crl := Get(base + "/ejbca/publicweb/webdist/certdist?cmd=crl&issuer=CN%3DSmokeCA%2CO%3DQuenchworks")
		if c != http.StatusOK || len(crl) < 100 {
			t.Fatalf("CRL download: %d, %d bytes", c, len(crl))
		}
		// 4447 = 0x115F; state 0A = LISTEN. Loopback is 0100007F.
		tcp, _ := docker("exec", name, "cat", "/proc/net/tcp", "/proc/net/tcp6")
		if !strings.Contains(tcp, "0100007F:115F") || strings.Contains(tcp, "00000000:115F") {
			t.Fatalf("the CLI listener must bind loopback only:\n%s", tcp)
		}
		t.Logf("smoke test passed (ejbca %s: read-only WildFly, ejbcahealth ALLOK, CA created, PEM certificate issued, CRL served, CLI listener on loopback; user %s)", t.Want, u)
	})
}
