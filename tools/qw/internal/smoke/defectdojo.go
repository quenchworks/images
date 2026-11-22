package smoke

import (
	"net/http"
	"strings"
	"time"
)

func init() {
	// Against the catalog's PostgreSQL and Valkey: the initializer migrates the database and creates the admin, then
	// uWSGI starts on a read-only root filesystem. The login page and a static file answer on the HTTP router (8081,
	// whitenoise), the admin gets an API token, a wrong password is refused, and the token reads the user list.
	register("defectdojo", func(t *T) {
		const base = "http://127.0.0.1:18119"
		const dbpw, adminpw = "Smoke-Db-2026", "Smoke-Dojo-Admin-2026!"
		u := t.User("1001")
		net := t.Name("net")
		t.Docker("network", "create", net)
		t.Cleanup(func() { docker("network", "rm", net) })
		db, kv := t.Name("db"), t.Name("kv")
		t.Docker("run", "-d", "--name", db, "--network", net, "--network-alias", "db", "-e", "POSTGRES_PASSWORD="+dbpw,
			"-e", "POSTGRES_USER=dojo", "-e", "POSTGRES_DB=defectdojo", leantimePostgres)
		t.Cleanup(func() { docker("rm", "-f", db) })
		t.Docker("run", "-d", "--name", kv, "--network", net, "--network-alias", "kv", "ghcr.io/quenchworks/images/valkey:9.1.3")
		t.Cleanup(func() { docker("rm", "-f", kv) })
		t.waitFor(60, 2*time.Second, db, "postgres did not become ready", func() bool {
			_, err := docker("exec", "-e", "PGPASSWORD="+dbpw, db, "psql", "-h", "127.0.0.1", "-U", "dojo", "-d", "defectdojo", "-tAc", "select 1")
			return err == nil
		})
		env := []string{"--network", net,
			"-e", "DD_DATABASE_URL=postgresql://dojo:" + dbpw + "@db:5432/defectdojo",
			"-e", "DD_CELERY_BROKER_URL=redis://kv:6379/0",
			"-e", "DD_SECRET_KEY=quench-smoke-secret-key-0123456789abcdef0123456789",
			"-e", "DD_CREDENTIAL_AES_256_KEY=quench-smoke-aes-key-0123456789",
			"-e", "DD_ADMIN_PASSWORD=" + adminpw}
		out, err := t.RunFlags(append([]string{"--entrypoint", "/entrypoint-initializer.sh", "--read-only", "--tmpfs", "/tmp:rw,mode=1777"}, env...))
		if err != nil {
			t.Fatalf("initializer failed: %v\n%s", err, out)
		}
		name := t.Name("")
		t.Start(name, append([]string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--tmpfs", "/var/run/defectdojo:rw,uid=1001,gid=1001",
			"--tmpfs", "/app/media:rw,uid=1001,gid=1001", "-p", "127.0.0.1:18119:8081"}, env...))
		t.waitFor(120, 2*time.Second, name, "the login page did not answer", func() bool {
			c, _ := Get(base + "/login")
			return c == http.StatusOK
		})
		_, page := Get(base + "/login")
		t.ContainsFold(page, "defectdojo", "login page")
		if c, _ := Get(base + "/static/dojo/css/tailwind-out.css"); c != http.StatusOK {
			t.Fatalf("static file not served (%d)", c)
		}
		j := map[string]string{"Content-Type": "application/json"}
		if c, _ := DoH("POST", base+"/api/v2/api-token-auth/", `{"username":"admin","password":"wrong-password-1"}`, j); c == http.StatusOK {
			t.Fatalf("a wrong password was accepted")
		}
		c, tok := DoH("POST", base+"/api/v2/api-token-auth/", `{"username":"admin","password":"`+adminpw+`"}`, j)
		if c != http.StatusOK || !strings.Contains(tok, `"token"`) {
			t.Fatalf("admin token: %d %s\n%s", c, tok, t.Logs(name))
		}
		key := tok[strings.Index(tok, `"token":"`)+9:]
		key = key[:strings.Index(key, `"`)]
		c, users := DoH("GET", base+"/api/v2/users/", "", map[string]string{"Authorization": "Token " + key})
		if c != http.StatusOK || !strings.Contains(users, `"username":"admin"`) {
			t.Fatalf("users with the token: %d %s", c, users)
		}
		t.Logf("smoke test passed (defectdojo %s: initializer, read-only uWSGI, login page, static files, API token, wrong password refused; user %s)", t.Want, u)
	})
}
