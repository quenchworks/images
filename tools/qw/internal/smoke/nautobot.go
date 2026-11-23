package smoke

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func init() {
	// Against the catalog's PostgreSQL and Valkey, on a read-only root with a data volume: post_upgrade migrates and
	// createsuperuser runs from the same image, uWSGI serves the login page and the static files, a wrong password gets
	// no API token, the admin's does, and a manufacturer created through the API reads back.
	register("nautobot", func(t *T) {
		const base = "http://127.0.0.1:18123"
		const dbpw, kvpw, adminpw = "Smoke-Db-2026", "Smoke-Kv-2026", "Smoke-Nautobot-Admin-2026!"
		u := t.User("1001")
		net := t.Name("net")
		t.Docker("network", "create", net)
		t.Cleanup(func() { docker("network", "rm", net) })
		db, kv := t.Name("db"), t.Name("kv")
		t.Docker("run", "-d", "--name", db, "--network", net, "--network-alias", "db", "-e", "POSTGRES_PASSWORD="+dbpw,
			"-e", "POSTGRES_USER=nautobot", "-e", "POSTGRES_DB=nautobot_db", leantimePostgres)
		t.Cleanup(func() { docker("rm", "-f", db) })
		t.Docker("run", "-d", "--name", kv, "--network", net, "--network-alias", "kv", "ghcr.io/quenchworks/images/valkey:9.1.3", "--requirepass", kvpw)
		t.Cleanup(func() { docker("rm", "-f", kv) })
		t.waitFor(60, 2*time.Second, db, "postgres did not become ready", func() bool {
			_, err := docker("exec", "-e", "PGPASSWORD="+dbpw, db, "psql", "-h", "127.0.0.1", "-U", "nautobot", "-d", "nautobot_db", "-tAc", "select 1")
			return err == nil
		})
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", "-f", vol) })
		env := []string{"--network", net, "--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/var/lib/nautobot",
			"-e", "NAUTOBOT_DB_HOST=db", "-e", "NAUTOBOT_DB_NAME=nautobot_db", "-e", "NAUTOBOT_DB_USER=nautobot", "-e", "NAUTOBOT_DB_PASSWORD=" + dbpw,
			"-e", "NAUTOBOT_REDIS_HOST=kv", "-e", "NAUTOBOT_REDIS_PASSWORD=" + kvpw, "-e", "NAUTOBOT_ALLOWED_HOSTS=*",
			"-e", "NAUTOBOT_SECRET_KEY=quench-smoke-secret-key-0123456789abcdef0123456789abcdef"}
		srv := "/opt/nautobot/venv/bin/nautobot-server"
		if out, err := t.RunFlags(append([]string{"--entrypoint", srv}, env...), "post_upgrade", "--no-collectstatic", "--no-send-installation-metrics"); err != nil {
			t.Fatalf("post_upgrade failed: %v\n%s", err, out)
		}
		if out, err := t.RunFlags(append([]string{"--entrypoint", srv, "-e", "DJANGO_SUPERUSER_PASSWORD=" + adminpw}, env...),
			"createsuperuser", "--no-input", "--username", "admin", "--email", "admin@example.com"); err != nil {
			t.Fatalf("createsuperuser failed: %v\n%s", err, out)
		}
		name := t.Name("")
		t.Start(name, append([]string{"-p", "127.0.0.1:18123:8080"}, env...))
		t.waitFor(120, 2*time.Second, name, "the login page did not answer", func() bool {
			c, _ := Get(base + "/login/")
			return c == http.StatusOK
		})
		_, page := Get(base + "/login/")
		t.ContainsFold(page, "nautobot", "login page")
		if c, _ := Get(base + "/static/img/favicon.ico"); c != http.StatusOK {
			t.Fatalf("static file not served (%d)", c)
		}
		basic := func(pw string) map[string]string {
			return map[string]string{"Content-Type": "application/json", "Accept": "application/json",
				"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:"+pw))}
		}
		if c, _ := DoH("POST", base+"/api/users/tokens/", `{}`, basic("wrong-password-1")); c == http.StatusCreated {
			t.Fatalf("a wrong password got a token")
		}
		c, body := DoH("POST", base+"/api/users/tokens/", `{}`, basic(adminpw))
		var tok struct {
			Key string `json:"key"`
		}
		if c != http.StatusCreated || json.Unmarshal([]byte(body), &tok) != nil || tok.Key == "" {
			t.Fatalf("token: %d %s\n%s", c, body, t.Logs(name))
		}
		h := map[string]string{"Content-Type": "application/json", "Accept": "application/json", "Authorization": "Token " + tok.Key}
		if c, b := DoH("POST", base+"/api/dcim/manufacturers/", `{"name":"Smoke Vendor"}`, h); c != http.StatusCreated {
			t.Fatalf("create manufacturer: %d %s", c, b)
		}
		c, list := DoH("GET", base+"/api/dcim/manufacturers/?name=Smoke%20Vendor", "", h)
		if c != http.StatusOK || !strings.Contains(list, `"name":"Smoke Vendor"`) {
			t.Fatalf("read manufacturer back: %d %s", c, list)
		}
		t.Logf("smoke test passed (nautobot %s: post_upgrade, createsuperuser, read-only uWSGI, static files, wrong password refused, token, manufacturer created and read back; user %s)", t.Want, u)
	})
}
