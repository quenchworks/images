package smoke

import (
	"net/http"
	"strings"
)

func init() {
	// On a read-only rootfs with our postgresql: the launcher runs prisma migrate deploy and starts the Next.js
	// server, /api/status reports the database connected and the version, the login page and a static asset are
	// served, the API refuses a request without a key or with a wrong one, and a restart re-runs the migrations.
	register("rallly", func(t *T) {
		const base = "http://127.0.0.1:18106"
		const dbpw = "smoke-db-pass"
		u := t.User("1001")
		net := t.Name("net")
		t.Docker("network", "create", net)
		t.Cleanup(func() { docker("network", "rm", net) })
		pg := t.Name("pg")
		t.Cleanup(func() { docker("rm", "-f", pg) })
		t.Docker("run", "-d", "--name", pg, "--network", net, "-e", "POSTGRES_PASSWORD="+dbpw, "-e", "POSTGRES_DB=rallly", "ghcr.io/quenchworks/images/postgresql:18.7")
		// migrate deploy fails on a refused connection: wait for the server initdb restarts into
		t.waitCount(pg, "ready to accept connections", 2, 60)
		start := func(name string) {
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--network", net, "-p", "127.0.0.1:18106:3000",
				"-e", "DATABASE_URL=postgresql://postgres:" + dbpw + "@" + pg + ":5432/rallly",
				"-e", "SECRET_PASSWORD=quench-smoke-secret-0123456789abcdef", "-e", "NEXT_PUBLIC_BASE_URL=" + base,
				"-e", "SUPPORT_EMAIL=smoke@quench-works.com"})
			t.WaitHTTP(base+"/api/status", 120, name)
		}
		status := func(name string) {
			c, b := Get(base + "/api/status")
			if c != http.StatusOK || !strings.Contains(b, `"database":"connected"`) {
				t.Fatalf("/api/status: %d %s\n%s", c, b, t.Logs(name))
			}
			if t.Want != "" {
				t.Contains(b, `"version":"v`+t.Want+`"`, "/api/status")
			}
		}
		name := t.Name("")
		start(name)
		status(name)
		t.Contains(t.Logs(name), "migrations", "the launcher's migrate deploy")
		c, page := Get(base + "/login")
		if c != http.StatusOK || !strings.Contains(page, "/_next/static/") {
			t.Fatalf("/login: %d", c)
		}
		i := strings.Index(page, "/_next/static/")
		asset := page[i : i+strings.IndexAny(page[i:], `"' `)]
		if c, _ := Get(base + asset); c != http.StatusOK {
			t.Fatalf("static asset %s: %d", asset, c)
		}
		if c, _ := Get(base + "/api/v1/polls"); c != http.StatusUnauthorized {
			t.Fatalf("an unkeyed API call answered %d, want 401", c)
		}
		if c, _ := DoH("GET", base+"/api/v1/polls", "", map[string]string{"Authorization": "Bearer sk_wrong_key_1"}); c != http.StatusUnauthorized {
			t.Fatalf("a wrong API key answered %d, want 401", c)
		}
		out := t.Docker("exec", "-e", "PGPASSWORD="+dbpw, pg, "psql", "-h", "127.0.0.1", "-U", "postgres", "-d", "rallly", "-tAc", "select count(*) from _prisma_migrations where finished_at is not null")
		if strings.TrimSpace(out) == "0" {
			t.Fatalf("no finished migrations")
		}
		docker("rm", "-f", name)
		again := t.Name("again")
		start(again)
		status(again)
		t.Contains(t.Logs(again), "No pending migrations", "migrate deploy on a restart")
		t.Logf("smoke test passed (rallly %s: read-only boot on postgresql, %s migrations applied, status connected, login page and static asset served, unkeyed and wrong-key API calls refused, clean restart; user %s)", t.Want, strings.TrimSpace(out), u)
	})
}
