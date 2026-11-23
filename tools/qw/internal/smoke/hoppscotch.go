package smoke

import (
	"net/http"
	"strings"
	"time"
)

func init() {
	// Against our PostgreSQL: quench-migrate.mjs (Prisma's migrations and _prisma_migrations bookkeeping on the pg
	// driver; Prisma's Rust schema engine is not shipped) runs twice, the second a no-op, then the API boots on a
	// read-only rootfs, /ping answers and GraphQL resolves a query.
	register("hoppscotch-backend", func(t *T) {
		const base = "http://127.0.0.1:13171"
		u := t.User("1001")
		net_, pg, pw := t.Name("net"), t.Name("pg"), t.Name("pw")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Cleanup(func() { docker("rm", "-f", pg) })
		t.Docker("run", "-d", "--name", pg, "--network", net_, "-e", "POSTGRES_PASSWORD="+pw, "-e", "POSTGRES_DB=hopp",
			"ghcr.io/quenchworks/images/postgresql:18.6")
		t.waitFor(60, 2*time.Second, pg, "postgres did not become ready", func() bool {
			_, err := docker("exec", "-e", "PGPASSWORD="+pw, pg, "psql", "-h", "127.0.0.1", "-U", "postgres", "-d", "hopp", "-tAc", "select 1")
			return err == nil
		})
		env := []string{"--network", net_, "--read-only", "--tmpfs", "/tmp:rw,mode=1777",
			"-e", "DATABASE_URL=postgresql://postgres:" + pw + "@" + pg + ":5432/hopp",
			"-e", "DATA_ENCRYPTION_KEY=" + strings.Repeat("k", 32),
			"-e", "WHITELISTED_ORIGINS=http://localhost:3000,http://localhost:3100", "-e", "TRUST_PROXY=false",
			"-e", "VITE_BASE_URL=http://localhost:3000", "-e", "VITE_SHORTCODE_BASE_URL=http://localhost:3000",
			"-e", "VITE_ADMIN_URL=http://localhost:3100", "-e", "VITE_BACKEND_GQL_URL=http://localhost:3170/graphql",
			"-e", "VITE_BACKEND_WS_URL=ws://localhost:3170/graphql", "-e", "VITE_BACKEND_API_URL=http://localhost:3170/v1"}
		for i := 0; i < 2; i++ {
			out, err := t.RunFlags(append(append([]string{}, env...), "--entrypoint", "/usr/bin/node"),
				"quench-migrate.mjs")
			want := "migration(s) applied"
			if i == 1 {
				want = "No pending migrations"
			}
			if err != nil || !strings.Contains(out, want) {
				t.Fatalf("quench-migrate run %d: %v\n%s", i+1, err, out)
			}
		}
		name := t.Name("")
		t.Start(name, append(append([]string{}, env...), "-p", "127.0.0.1:13171:3170"))
		t.waitCode(base+"/ping", 60, 2*time.Second, name, 200)
		req, _ := http.NewRequest("POST", base+"/graphql", strings.NewReader(`{"query":"{ __typename }"}`))
		req.Header.Set("Content-Type", "application/json") // Apollo's CSRF guard refuses other content types
		c, b := insecureDo(req)
		if c != 200 || !strings.Contains(b, `"__typename":"Query"`) {
			t.Fatalf("graphql: %d %s\n%s", c, b, t.Logs(name))
		}
		if logs := t.Logs(name); strings.Contains(logs, "ERROR") {
			t.Fatalf("errors in the log:\n%s", logs)
		}
		t.Logf("smoke test passed (hoppscotch-backend %s: migrations applied, second run a no-op, read-only boot, /ping, GraphQL; user %s)", t.Want, u)
	})
}
