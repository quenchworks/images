package smoke

import (
	"strings"
	"time"
)

func init() {
	// Against our PostgreSQL and ClickHouse: createdb, then the migrations twice (the second a no-op), then the
	// release boots on a read-only rootfs and /api/health reports both stores ok.
	register("plausible", func(t *T) {
		const base = "http://127.0.0.1:18000"
		u := t.User("1001")
		net_, pg, ch, pw := t.Name("net"), t.Name("pg"), t.Name("ch"), t.Name("pw")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Cleanup(func() { docker("rm", "-f", pg, ch) })
		t.Docker("run", "-d", "--name", pg, "--network", net_, "-e", "POSTGRES_PASSWORD="+pw, "-e", "POSTGRES_DB=plausible_db",
			"ghcr.io/quenchworks/images/postgresql:18.6")
		t.Docker("run", "-d", "--name", ch, "--network", net_, "-e", "CLICKHOUSE_USER=plausible", "-e", "CLICKHOUSE_PASSWORD="+pw,
			"ghcr.io/quenchworks/images/clickhouse:26.10.10.3")
		t.waitFor(60, 2*time.Second, pg, "postgres did not become ready", func() bool {
			_, err := docker("exec", "-e", "PGPASSWORD="+pw, pg, "psql", "-h", "127.0.0.1", "-U", "postgres", "-d", "plausible_db", "-tAc", "select 1")
			return err == nil
		})
		t.waitFor(60, 2*time.Second, ch, "clickhouse did not become ready", func() bool {
			_, err := docker("exec", ch, "clickhouse", "client", "--user", "plausible", "--password", pw, "--query", "SELECT 1")
			return err == nil
		})
		env := []string{"--network", net_, "--read-only", "--tmpfs", "/tmp:rw,mode=1777,exec", "--tmpfs", "/var/lib/plausible:rw,mode=1777",
			"-e", "BASE_URL=http://localhost:18000", "-e", "SECRET_KEY_BASE=" + strings.Repeat("s", 64),
			"-e", "DATABASE_URL=postgres://postgres:" + pw + "@" + pg + ":5432/plausible_db",
			"-e", "CLICKHOUSE_DATABASE_URL=http://plausible:" + pw + "@" + ch + ":8123/plausible_events_db"}
		eval := func(fn string) string {
			out, err := t.RunFlags(append(append([]string{}, env...), "--entrypoint", "/opt/plausible/bin/plausible"), "eval", "Plausible.Release."+fn)
			if err != nil {
				t.Fatalf("%s: %v\n%s", fn, err, out)
			}
			return out
		}
		eval("createdb")
		eval("interweave_migrate")
		if out := eval("interweave_migrate"); strings.Contains(strings.ToLower(out), "error") {
			t.Fatalf("second migration run:\n%s", out)
		}
		name := t.Name("")
		t.Start(name, append(append([]string{}, env...), "-p", "127.0.0.1:18000:8000"))
		t.waitCode(base+"/api/health", 120, 2*time.Second, name, 200)
		_, b := restDo("GET", base+"/api/health", "", "", "")
		for _, k := range []string{`"postgres":"ok"`, `"clickhouse":"ok"`} {
			if !strings.Contains(b, k) {
				t.Fatalf("/api/health lacks %s: %s\n%s", k, b, t.Logs(name))
			}
		}
		if c, _ := restDo("GET", base+"/register", "", "", ""); c != 200 {
			t.Fatalf("/register got %d\n%s", c, t.Logs(name))
		}
		t.Logf("smoke test passed (plausible %s: createdb, migrations twice, read-only boot, /api/health postgres+clickhouse ok, /register; user %s)", t.Want, u)
	})
}
