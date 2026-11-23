package smoke

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func init() {
	// Against the catalog's PostgreSQL and Valkey, on a read-only root filesystem: migrate and createsuperuser run
	// from the same image, gunicorn serves the login page and whitenoise the static CSS, the admin provisions an API
	// token (a wrong password is refused), and a site created through the API reads back.
	register("netbox", func(t *T) {
		const base = "http://127.0.0.1:18121"
		const dbpw, kvpw, adminpw = "Smoke-Db-2026", "Smoke-Kv-2026", "Smoke-NetBox-Admin-2026!"
		u := t.User("1001")
		net := t.Name("net")
		t.Docker("network", "create", net)
		t.Cleanup(func() { docker("network", "rm", net) })
		db, kv := t.Name("db"), t.Name("kv")
		t.Docker("run", "-d", "--name", db, "--network", net, "--network-alias", "db", "-e", "POSTGRES_PASSWORD="+dbpw,
			"-e", "POSTGRES_USER=netbox", "-e", "POSTGRES_DB=netbox_db", leantimePostgres)
		t.Cleanup(func() { docker("rm", "-f", db) })
		t.Docker("run", "-d", "--name", kv, "--network", net, "--network-alias", "kv", "ghcr.io/quenchworks/images/valkey:9.1.3", "--requirepass", kvpw)
		t.Cleanup(func() { docker("rm", "-f", kv) })
		t.waitFor(60, 2*time.Second, db, "postgres did not become ready", func() bool {
			_, err := docker("exec", "-e", "PGPASSWORD="+dbpw, db, "psql", "-h", "127.0.0.1", "-U", "netbox", "-d", "netbox_db", "-tAc", "select 1")
			return err == nil
		})
		env := []string{"--network", net, "--read-only", "--tmpfs", "/tmp:rw,mode=1777",
			"-e", "DB_HOST=db", "-e", "DB_NAME=netbox_db", "-e", "DB_USER=netbox", "-e", "DB_PASSWORD=" + dbpw,
			"-e", "REDIS_HOST=kv", "-e", "REDIS_PASSWORD=" + kvpw,
			"-e", "SECRET_KEY=quench-smoke-secret-key-0123456789abcdef0123456789abcdef",
			"-e", "API_TOKEN_PEPPER=quench-smoke-pepper-0123456789abcdef0123456789abcdef"}
		py := "/opt/netbox/venv/bin/python"
		if out, err := t.RunFlags(append([]string{"--entrypoint", py}, env...), "manage.py", "migrate", "--no-input"); err != nil {
			t.Fatalf("migrate failed: %v\n%s", err, out)
		}
		if out, err := t.RunFlags(append([]string{"--entrypoint", py, "-e", "DJANGO_SUPERUSER_PASSWORD=" + adminpw}, env...),
			"manage.py", "createsuperuser", "--no-input", "--username", "admin", "--email", "admin@example.com"); err != nil {
			t.Fatalf("createsuperuser failed: %v\n%s", err, out)
		}
		name := t.Name("")
		t.Start(name, append([]string{"-p", "127.0.0.1:18121:8080"}, env...))
		t.waitFor(90, 2*time.Second, name, "the login page did not answer", func() bool {
			c, _ := Get(base + "/login/")
			return c == http.StatusOK
		})
		_, page := Get(base + "/login/")
		t.ContainsFold(page, "netbox", "login page")
		if c, _ := Get(base + "/static/netbox.css"); c != http.StatusOK {
			t.Fatalf("static CSS not served (%d)", c)
		}
		j := map[string]string{"Content-Type": "application/json", "Accept": "application/json"}
		if c, _ := DoH("POST", base+"/api/users/tokens/provision/", `{"username":"admin","password":"wrong-password-1"}`, j); c == http.StatusCreated {
			t.Fatalf("a wrong password got a token")
		}
		c, body := DoH("POST", base+"/api/users/tokens/provision/", `{"username":"admin","password":"`+adminpw+`"}`, j)
		var tok struct {
			Version int    `json:"version"`
			Key     string `json:"key"`
			Token   string `json:"token"`
		}
		if c != http.StatusCreated || json.Unmarshal([]byte(body), &tok) != nil || tok.Token == "" {
			t.Fatalf("token provision: %d %s\n%s", c, body, t.Logs(name))
		}
		auth := "Token " + tok.Token
		if tok.Version == 2 {
			auth = "Bearer nbt_" + tok.Key + "." + tok.Token
		}
		h := map[string]string{"Content-Type": "application/json", "Accept": "application/json", "Authorization": auth}
		if c, b := DoH("POST", base+"/api/dcim/sites/", `{"name":"Smoke Site","slug":"smoke-site","status":"active"}`, h); c != http.StatusCreated {
			t.Fatalf("create site: %d %s", c, b)
		}
		c, sites := DoH("GET", base+"/api/dcim/sites/?slug=smoke-site", "", h)
		if c != http.StatusOK || !strings.Contains(sites, `"name":"Smoke Site"`) {
			t.Fatalf("read site back: %d %s", c, sites)
		}
		t.Logf("smoke test passed (netbox %s: migrate, createsuperuser, read-only gunicorn, static via whitenoise, token v%d, wrong password refused, site created and read back; user %s)", t.Want, tok.Version, u)
	})
}
