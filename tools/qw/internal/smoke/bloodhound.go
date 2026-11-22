package smoke

import (
	"net/http"
	"strings"
	"time"
)

func init() {
	// On a read-only root filesystem against the catalog's PostgreSQL, with the graph in PostgreSQL too (pg graph
	// driver): the migrations run, the embedded UI answers, the default admin signs in, a wrong password is refused,
	// and the session token reads the signed-in user.
	register("bloodhound", func(t *T) {
		const base = "http://127.0.0.1:18126"
		const dbpw, adminpw = "Smoke-Db-2026", "Smoke-BloodHound-Admin-2026!"
		u := t.User("1001")
		net := t.Name("net")
		t.Docker("network", "create", net)
		t.Cleanup(func() { docker("network", "rm", net) })
		db := t.Name("db")
		t.Docker("run", "-d", "--name", db, "--network", net, "--network-alias", "db", "-e", "POSTGRES_PASSWORD="+dbpw,
			"-e", "POSTGRES_USER=bloodhound", "-e", "POSTGRES_DB=bloodhound_db", leantimePostgres)
		t.Cleanup(func() { docker("rm", "-f", db) })
		t.waitFor(60, 2*time.Second, db, "postgres did not become ready", func() bool {
			_, err := docker("exec", "-e", "PGPASSWORD="+dbpw, db, "psql", "-h", "127.0.0.1", "-U", "bloodhound", "-d", "bloodhound_db", "-tAc", "select 1")
			return err == nil
		})
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--tmpfs", "/opt/bloodhound/work:rw,uid=1001,gid=1001",
			"--network", net, "-p", "127.0.0.1:18126:8080",
			"-e", "bhe_database_connection=user=bloodhound password=" + dbpw + " dbname=bloodhound_db host=db",
			"-e", "bhe_graph_driver=pg",
			"-e", "bhe_root_url=http://127.0.0.1:18126/",
			"-e", "bhe_default_admin_principal_name=admin",
			"-e", "bhe_default_admin_password=" + adminpw,
			"-e", "bhe_default_admin_expire_now=false"})
		t.waitFor(120, 2*time.Second, name, "the UI did not answer", func() bool {
			c, _ := Get(base + "/ui/")
			return c == http.StatusOK
		})
		j := map[string]string{"Content-Type": "application/json"}
		if c, _ := DoH("POST", base+"/api/v2/login", `{"login_method":"secret","username":"admin","secret":"wrong-password-1"}`, j); c == http.StatusOK {
			t.Fatalf("a wrong password was accepted")
		}
		c, b := DoH("POST", base+"/api/v2/login", `{"login_method":"secret","username":"admin","secret":"`+adminpw+`"}`, j)
		if c != http.StatusOK || !strings.Contains(b, `"session_token":"`) {
			t.Fatalf("admin login: %d %s\n%s", c, b, t.Logs(name))
		}
		tok := b[strings.Index(b, `"session_token":"`)+17:]
		tok = tok[:strings.Index(tok, `"`)]
		c, self := DoH("GET", base+"/api/v2/self", "", map[string]string{"Authorization": "Bearer " + tok})
		if c != http.StatusOK || !strings.Contains(self, `"principal_name":"admin"`) {
			t.Fatalf("self with the session token: %d %s", c, self)
		}
		_, v := DoH("GET", base+"/api/version", "", map[string]string{"Authorization": "Bearer " + tok})
		t.WantVersion(v)
		t.Logf("smoke test passed (bloodhound %s: read-only boot on PostgreSQL with the pg graph driver, embedded UI, admin login, wrong password refused, session token; user %s)", t.Want, u)
	})
}
