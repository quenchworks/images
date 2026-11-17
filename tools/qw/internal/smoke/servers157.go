package smoke

import (
	"net/http"
	"strings"
)

func init() {
	// On a read-only rootfs with our postgresql: Wiki.js connects, creates its schema and answers /_live, the web UI
	// is served, the database holds Wiki.js's tables, and a restart against the same database comes back.
	register("wikijs", func(t *T) {
		const base = "http://127.0.0.1:18111"
		const dbpw = "smoke-db-pass"
		u := t.User("1001")
		net := t.Name("net")
		t.Docker("network", "create", net)
		t.Cleanup(func() { docker("network", "rm", net) })
		pg := t.Name("pg")
		t.Cleanup(func() { docker("rm", "-f", pg) })
		t.Docker("run", "-d", "--name", pg, "--network", net, "-e", "POSTGRES_PASSWORD="+dbpw, "-e", "POSTGRES_DB=wiki", "ghcr.io/quenchworks/images/postgresql:18.7")
		t.waitCount(pg, "ready to accept connections", 2, 60)
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		start := func(name string) {
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--network", net, "-v", vol + ":/wiki/data",
				"-e", "DB_HOST=" + pg, "-e", "DB_USER=postgres", "-e", "DB_PASS=" + dbpw, "-e", "DB_NAME=wiki",
				"-p", "127.0.0.1:18111:3000"})
			t.WaitHTTP(base+"/_live", 150, name)
		}
		name := t.Name("")
		start(name)
		c, page := Get(base + "/")
		if c != http.StatusOK || !strings.Contains(strings.ToLower(page), "<html") {
			t.Fatalf("GET /: %d\n%s", c, t.Logs(name))
		}
		out := t.Docker("exec", "-e", "PGPASSWORD="+dbpw, pg, "psql", "-h", "127.0.0.1", "-U", "postgres", "-d", "wiki", "-tAc",
			"select count(*) from information_schema.tables where table_schema = 'wiki'")
		if n := strings.TrimSpace(out); n == "0" || n == "" {
			t.Fatalf("no Wiki.js tables in schema wiki\n%s", t.Logs(name))
		}
		docker("rm", "-f", name)
		again := t.Name("again")
		start(again)
		if c, _ := Get(base + "/"); c != http.StatusOK {
			t.Fatalf("GET / after a restart: %d\n%s", c, t.Logs(again))
		}
		t.Logf("smoke test passed (wikijs %s: read-only boot on postgresql, /_live, web UI, %s tables in schema wiki, clean restart; user %s)", t.Want, strings.TrimSpace(out), u)
	})
}
