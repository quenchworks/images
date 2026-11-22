package smoke

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var artexSetupToken = regexp.MustCompile(`Setup token for the /setup page: (\S+)`)

// artexDo sends one JSON request to ARTEX, with the session JWT as a bearer token when given.
func artexDo(method, url, tok, body string) (int, string) {
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	return insecureDo(req)
}

func init() {
	// On a read-only rootfs against the catalog's PostgreSQL: the schema auto-creates, the embedded UI is served,
	// the one-time setup token from stderr sets the admin password (a wrong token is refused), the admin signs in
	// (a wrong password is refused) and /api/health reports the version, and after a restart the password and the
	// JWT keys on the volume still work.
	register("artex", func(t *T) {
		const base = "http://127.0.0.1:18115"
		const dbpw = "Smoke-Db-2026"
		const pw = "Smoke-Artex-Admin-2026"
		u := t.User("1001")
		net := t.Name("net")
		t.Docker("network", "create", net)
		t.Cleanup(func() { docker("network", "rm", net) })
		db := t.Name("db")
		t.Docker("run", "-d", "--name", db, "--network", net, "--network-alias", "db", "-e", "POSTGRES_PASSWORD="+dbpw,
			"-e", "POSTGRES_USER=artex", "-e", "POSTGRES_DB=artex_db", leantimePostgres)
		t.Cleanup(func() { docker("rm", "-f", db) })
		// ARTEX exits when PostgreSQL refuses the first connection, so wait for it first
		t.waitFor(60, 2*time.Second, db, "postgres did not become ready", func() bool {
			_, err := docker("exec", "-e", "PGPASSWORD="+dbpw, db, "psql", "-h", "127.0.0.1", "-U", "artex", "-d", "artex_db", "-tAc", "select 1")
			return err == nil
		})
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		flags := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--network", net, "-v", vol + ":/var/lib/artex",
			"-p", "127.0.0.1:18115:8787", "-e", "ARTEX_PG_DSN=postgres://artex:" + dbpw + "@db:5432/artex_db?sslmode=disable"}
		start := func(name string) {
			t.Start(name, flags)
			t.waitFor(90, 2*time.Second, name, "artex did not answer /api/ready", func() bool {
				c, _ := artexDo("GET", base+"/api/ready", "", "")
				return c == http.StatusOK
			})
		}
		login := func(p string) (int, string) {
			c, b := artexDo("POST", base+"/api/auth/login", "", `{"username":"ARTEX","password":"`+p+`"}`)
			var r struct {
				Token string `json:"token"`
			}
			_ = json.Unmarshal([]byte(b), &r)
			return c, r.Token
		}
		name := t.Name("")
		start(name)
		_, page := artexDo("GET", base+"/", "", "")
		t.ContainsFold(page, "artex", "the embedded UI")
		_, st := artexDo("GET", base+"/api/auth/status", "", "")
		t.Contains(st, `"initialized":false`, "auth status before setup")
		m := artexSetupToken.FindStringSubmatch(t.Logs(name))
		if m == nil {
			t.Fatalf("no setup token on stderr:\n%s", t.Logs(name))
		}
		if c, _ := artexDo("POST", base+"/api/auth/init", "", `{"password":"`+pw+`","setup_token":"wrong-token"}`); c != http.StatusForbidden {
			t.Fatalf("init with a wrong setup token got %d, want 403", c)
		}
		if c, b := artexDo("POST", base+"/api/auth/init", "", `{"password":"`+pw+`","setup_token":"`+m[1]+`"}`); c != http.StatusOK {
			t.Fatalf("init with the setup token: %d %s", c, b)
		}
		if c, _ := login("wrong-password-12345"); c == http.StatusOK {
			t.Fatalf("a wrong password was accepted")
		}
		health := func() {
			c, tok := login(pw)
			if c != http.StatusOK || tok == "" {
				t.Fatalf("admin login: %d", c)
			}
			_, h := artexDo("GET", base+"/api/health", tok, "")
			t.Contains(h, `"version":"`+t.Want+`"`, "/api/health signed in")
		}
		health()
		docker("rm", "-f", name)
		start(t.Name("again"))
		_, st = artexDo("GET", base+"/api/auth/status", "", "")
		t.Contains(st, `"initialized":true`, "auth status after a restart")
		health()
		t.Logf("smoke test passed (artex %s: read-only boot against PostgreSQL, embedded UI served, setup token sets the admin password and a wrong token is refused, wrong password refused, signed-in /api/health reports the version, kept across a restart; user %s)", t.Want, u)
	})
}
