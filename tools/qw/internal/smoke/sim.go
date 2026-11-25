package smoke

import (
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"
)

// simDB starts our PostgreSQL (it ships pgvector) on a fresh network and runs the given sim-migrations image against
// it twice (the second run proves the migrations are idempotent). It returns the network, the DATABASE_URL and the
// postgres container name (its password is the name's "pw" twin, see simPSQL).
// simKV starts a Valkey on net_ and returns its URL: 0.9.66+ refuses to boot without REDIS_URL (sim's instrumentation
// hook and realtime's env schema both require it).
func (t *T) simKV(net_ string) string {
	kv := t.Name("kv")
	t.Cleanup(func() { docker("rm", "-f", kv) })
	// the catalog's Valkey runs protected mode: without a password it closes every non-loopback client
	t.Docker("run", "-d", "--name", kv, "--network", net_, "ghcr.io/quenchworks/images/valkey:9.1.3", "--requirepass", "smoke-kv-pw")
	return "redis://:smoke-kv-pw@" + kv + ":6379"
}

func (t *T) simDB(migrations string) (string, string, string) {
	net_, pg, pw := t.Name("net"), t.Name("pg"), t.Name("pw")
	t.Docker("network", "create", net_)
	t.Cleanup(func() { docker("network", "rm", net_) })
	t.Cleanup(func() { docker("rm", "-f", pg) })
	t.Docker("run", "-d", "--name", pg, "--network", net_, "-e", "POSTGRES_PASSWORD="+pw, "-e", "POSTGRES_DB=sim",
		"ghcr.io/quenchworks/images/postgresql:18.6")
	t.waitFor(60, 2*time.Second, pg, "postgres did not become ready", func() bool {
		_, err := docker("exec", "-e", "PGPASSWORD="+pw, pg, "psql", "-h", "127.0.0.1", "-U", "postgres", "-d", "sim", "-tAc", "select 1")
		return err == nil
	})
	url := "postgresql://postgres:" + pw + "@" + pg + ":5432/sim"
	for i := 0; i < 2; i++ {
		if out, err := docker("run", "--rm", "--network", net_, "--read-only", "--tmpfs", "/tmp", "-e", "DATABASE_URL="+url,
			"-e", "ENCRYPTION_KEY="+strings.Repeat("e", 64), migrations); err != nil {
			t.Fatalf("migrations run %d failed: %v\n%s", i+1, err, out)
		}
	}
	return net_, url, pg
}

func (t *T) simPSQL(pg, sql string) string {
	out, err := docker("exec", "-e", "PGPASSWORD="+t.Name("pw"), pg, "psql", "-h", "127.0.0.1", "-U", "postgres", "-d", "sim", "-tAc", sql)
	if err != nil {
		t.Fatalf("psql %q: %v\n%s", sql, err, out)
	}
	return strings.TrimSpace(out)
}

func init() {
	// Migrates an empty PostgreSQL twice on a read-only rootfs: the schema lands (user table, pgvector extension) and the
	// second run changes nothing.
	register("sim-migrations", func(t *T) {
		t.User("1001")
		_, _, pg := t.simDB(t.Image)
		t.Contains(t.simPSQL(pg, "select count(*) from information_schema.tables where table_name='user'"), "1", "user table")
		t.Contains(t.simPSQL(pg, "select extname from pg_extension where extname='vector'"), "vector", "pgvector extension")
		t.Logf("smoke test passed (schema migrated twice, pgvector on)")
	})

	// The socket server on a read-only rootfs against a migrated database: /health answers and a socket.io polling
	// handshake opens an Engine.IO session.
	register("sim-realtime", func(t *T) {
		const b = "http://127.0.0.1:13302"
		u := t.User("1001")
		ver := t.Want
		if ver == "" {
			ver = t.LabelVersion()
		}
		net_, url, _ := t.simDB("ghcr.io/quenchworks/images/sim-migrations:" + ver)
		redis := t.simKV(net_)
		name := t.Name("")
		t.Start(name, []string{"--network", net_, "--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-p", "127.0.0.1:13302:3002", "-e", "REDIS_URL=" + redis,
			"-e", "DATABASE_URL=" + url, "-e", "NEXT_PUBLIC_APP_URL=http://localhost:3000", "-e", "BETTER_AUTH_URL=http://localhost:3000",
			"-e", "BETTER_AUTH_SECRET=" + strings.Repeat("a", 64), "-e", "INTERNAL_API_SECRET=" + strings.Repeat("i", 64)})
		t.waitCode(b+"/health", 60, time.Second, name, 200)
		_, hs := Get(b + "/socket.io/?EIO=4&transport=polling")
		t.Contains(hs, `"sid":"`, "engine.io handshake")
		t.Logf("smoke test passed (uid %s, health, socket.io handshake)", u)
	})

	// The app on a read-only rootfs against a migrated database (sim-migrations of the same tag): health answers, the
	// login page renders, an API route refuses an anonymous caller, and email sign-up then sign-in give a session.
	register("sim", func(t *T) {
		const b = "http://127.0.0.1:13300"
		u := t.User("1001")
		ver := t.Want
		if ver == "" {
			ver = t.LabelVersion()
		}
		net_, url, pg := t.simDB("ghcr.io/quenchworks/images/sim-migrations:" + ver)
		redis := t.simKV(net_)
		name := t.Name("")
		t.Start(name, []string{"--network", net_, "--read-only", "--tmpfs", "/tmp:rw,mode=1777",
			"--tmpfs", "/app/apps/sim/.next/cache:uid=1001,gid=1001", "--tmpfs", "/app/apps/sim/.next/server/route-cache:uid=1001,gid=1001",
			"-p", "127.0.0.1:13300:3000",
			"-e", "DATABASE_URL=" + url, "-e", "NEXT_PUBLIC_APP_URL=http://localhost:3000", "-e", "BETTER_AUTH_URL=http://localhost:3000",
			"-e", "BETTER_AUTH_SECRET=" + strings.Repeat("a", 64), "-e", "ENCRYPTION_KEY=" + strings.Repeat("e", 64),
			"-e", "INTERNAL_API_SECRET=" + strings.Repeat("i", 64), "-e", "REDIS_URL=" + redis})
		t.waitCode(b+"/api/health", 120, time.Second, name, 200)
		if code, _ := Get(b + "/login"); code != 200 {
			t.Fatalf("/login answered %d\n%s", code, t.Logs(name))
		}
		if code, _ := Get(b + "/api/workflows"); code != 401 {
			t.Fatalf("anonymous /api/workflows answered %d, want 401", code)
		}
		jar, _ := cookiejar.New(nil)
		c := &http.Client{Jar: jar, Timeout: 30 * time.Second}
		post := func(path, body string) int {
			req, _ := http.NewRequest("POST", b+path, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", "http://localhost:3000")
			r, err := c.Do(req)
			if err != nil {
				return 0
			}
			r.Body.Close()
			return r.StatusCode
		}
		const creds = `{"email":"smoke@example.com","password":"Smoke-Sim-2026!pw"`
		if code := post("/api/auth/sign-up/email", creds+`,"name":"Smoke"}`); code != 200 {
			t.Fatalf("sign-up answered %d\n%s", code, t.Logs(name))
		}
		t.Contains(t.simPSQL(pg, "select count(*) from \"user\" where email='smoke@example.com'"), "1", "user row")
		if code := post("/api/auth/sign-in/email", `{"email":"smoke@example.com","password":"wrong-password-0"}`); code < 400 {
			t.Fatalf("a wrong password got HTTP %d", code)
		}
		if code := post("/api/auth/sign-in/email", creds+"}"); code != 200 {
			t.Fatalf("sign-in answered %d", code)
		}
		t.Logf("smoke test passed (uid %s, health, login page, 401 anonymous, sign-up + sign-in)", u)
	})
}
