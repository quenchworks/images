package smoke

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func init() {
	// Against the catalog's PostgreSQL, on a read-only root: Liquibase migrates at start, the initial-setup API creates
	// the admin, a wrong password is refused by the API, and a feed served from the host (local-address blocking off
	// for the test) is subscribed and its entry read back through the REST API.
	register("commafeed", func(t *T) {
		const base = "http://127.0.0.1:18125"
		const dbpw, adminpw = "Smoke-Db-2026", "Smoke-CommaFeed-2026!"
		u := t.User("1001")
		feed := t.hostStub(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/rss+xml")
			fmt.Fprint(w, `<?xml version="1.0"?><rss version="2.0"><channel><title>Quench smoke feed</title><link>http://example.test/</link><description>d</description>`+
				`<item><title>quench smoke entry</title><link>http://example.test/1</link><guid>quench-smoke-1</guid><description>hello</description></item></channel></rss>`)
		}))
		net := t.Name("net")
		t.Docker("network", "create", net)
		t.Cleanup(func() { docker("network", "rm", net) })
		db := t.Name("db")
		t.Docker("run", "-d", "--name", db, "--network", net, "--network-alias", "db", "-e", "POSTGRES_PASSWORD="+dbpw,
			"-e", "POSTGRES_USER=commafeed", "-e", "POSTGRES_DB=commafeed_db", leantimePostgres)
		t.Cleanup(func() { docker("rm", "-f", db) })
		t.waitFor(60, 2*time.Second, db, "postgres did not become ready", func() bool {
			_, err := docker("exec", "-e", "PGPASSWORD="+dbpw, db, "psql", "-h", "127.0.0.1", "-U", "commafeed", "-d", "commafeed_db", "-tAc", "select 1")
			return err == nil
		})
		name := t.Name("")
		t.Start(name, append([]string{"--network", net, "--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-p", "127.0.0.1:18125:8082",
			"-e", "QUARKUS_DATASOURCE_JDBC_URL=jdbc:postgresql://db:5432/commafeed_db", "-e", "QUARKUS_DATASOURCE_USERNAME=commafeed",
			"-e", "QUARKUS_DATASOURCE_PASSWORD=" + dbpw, "-e", "QUARKUS_HTTP_AUTH_SESSION_ENCRYPTION_KEY=quench-smoke-session-key-0123456789",
			"-e", "COMMAFEED_HTTP_CLIENT_BLOCK_LOCAL_ADDRESSES=false"}, hostStubFlag...))
		t.waitFor(120, 2*time.Second, name, "the server info API did not answer", func() bool {
			c, b := Get(base + "/rest/server/get")
			return c == http.StatusOK && strings.Contains(b, `"initialSetupRequired":true`)
		})
		j := map[string]string{"Content-Type": "application/json", "Accept": "application/json"}
		if c, b := DoH("POST", base+"/rest/user/initialSetup", `{"name":"admin","password":"`+adminpw+`","email":"admin@example.com"}`, j); c != http.StatusOK {
			t.Fatalf("initial setup: %d %s\n%s", c, b, t.Logs(name))
		}
		auth := func(pw string) map[string]string {
			return map[string]string{"Content-Type": "application/json", "Accept": "application/json",
				"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:"+pw))}
		}
		if c, _ := DoH("GET", base+"/rest/user/profile", "", auth("wrong-password-1")); c == http.StatusOK {
			t.Fatalf("a wrong password was accepted")
		}
		if c, b := DoH("GET", base+"/rest/user/profile", "", auth(adminpw)); c != http.StatusOK || !strings.Contains(b, `"name":"admin"`) {
			t.Fatalf("profile with the admin password: %d %s", c, b)
		}
		if c, b := DoH("POST", base+"/rest/feed/subscribe", `{"url":"`+feed+`/feed.xml","title":"Quench smoke feed"}`, auth(adminpw)); c != http.StatusOK {
			t.Fatalf("subscribe: %d %s\n%s", c, b, t.Logs(name))
		}
		var entries string
		t.waitFor(60, 2*time.Second, name, "the feed entry never appeared", func() bool {
			_, entries = DoH("GET", base+"/rest/category/entries?id=all&readType=all", "", auth(adminpw))
			return strings.Contains(entries, "quench smoke entry")
		})
		t.Logf("smoke test passed (commafeed %s: Liquibase migration, initial setup, read-only root, wrong password refused, feed subscribed and its entry read back; user %s)", t.Want, u)
	})
}
