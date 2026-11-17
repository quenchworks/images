package smoke

import (
	"encoding/json"
	"net/http"
	"strings"
)

func init() {
	// On a read-only rootfs with our postgresql and valkey: start.mjs migrates, seeds and starts the server, /api/v1/health
	// is green, the client is served, the first signup returns an access token that logs in (a wrong one is refused),
	// the session reads the user, and the access token still logs in after a restart.
	register("ghostfolio", func(t *T) {
		const base = "http://127.0.0.1:18113"
		const dbpw = "smoke-db-pass"
		u := t.User("1001")
		net := t.Name("net")
		t.Docker("network", "create", net)
		t.Cleanup(func() { docker("network", "rm", net) })
		pg, kv := t.Name("pg"), t.Name("kv")
		t.Cleanup(func() { docker("rm", "-f", pg, kv) })
		t.Docker("run", "-d", "--name", pg, "--network", net, "-e", "POSTGRES_PASSWORD="+dbpw, "-e", "POSTGRES_DB=ghostfolio", "ghcr.io/quenchworks/images/postgresql:18.7")
		// valkey in protected mode drops non-loopback clients without a password: give it one, as the chart does
		t.Docker("run", "-d", "--name", kv, "--network", net, "ghcr.io/quenchworks/images/valkey:9.1.3", "--requirepass", "smoke-kv-pass")
		t.waitCount(pg, "ready to accept connections", 2, 60)
		start := func(name string) {
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--network", net, "-p", "127.0.0.1:18113:3333",
				"-e", "DATABASE_URL=postgresql://postgres:" + dbpw + "@" + pg + ":5432/ghostfolio",
				"-e", "REDIS_HOST=" + kv, "-e", "REDIS_PORT=6379", "-e", "REDIS_PASSWORD=smoke-kv-pass",
				"-e", "ACCESS_TOKEN_SALT=quench-smoke-salt-0123456789", "-e", "JWT_SECRET_KEY=quench-smoke-jwt-0123456789abcdef"})
			t.WaitHTTP(base+"/api/v1/health", 180, name)
		}
		j := map[string]string{"Content-Type": "application/json"}
		login := func(token string) (int, string) {
			c, b := DoH("POST", base+"/api/v1/auth/anonymous", `{"accessToken":"`+token+`"}`, j)
			var r struct{ AuthToken string }
			json.Unmarshal([]byte(b), &r)
			return c, r.AuthToken
		}
		name := t.Name("")
		start(name)
		if c, b := Get(base + "/api/v1/health"); c != http.StatusOK || !strings.Contains(b, "OK") {
			t.Fatalf("/api/v1/health: %d %s", c, b)
		}
		if c, page := Get(base + "/en/"); c != http.StatusOK || !strings.Contains(strings.ToLower(page), "<html") {
			t.Fatalf("the client: %d", c)
		}
		c, b := DoH("POST", base+"/api/v1/user", `{}`, j)
		var su struct{ AccessToken, AuthToken, Role string }
		if c/100 != 2 || json.Unmarshal([]byte(b), &su) != nil || su.AccessToken == "" {
			t.Fatalf("signup: %d %s\n%s", c, b, t.Logs(name))
		}
		if su.Role != "ADMIN" {
			t.Fatalf("the first account is %q, want ADMIN", su.Role)
		}
		c, tok := login(su.AccessToken)
		if c/100 != 2 || tok == "" {
			t.Fatalf("login with the access token: %d", c)
		}
		if c, b := DoH("GET", base+"/api/v1/user", "", map[string]string{"Authorization": "Bearer " + tok}); c != http.StatusOK || !strings.Contains(b, `"id":"`) {
			t.Fatalf("read the user: %d %s", c, b)
		}
		if c, tok := login("wrong-access-token-1"); c/100 == 2 && tok != "" {
			t.Fatalf("a wrong access token logged in")
		}
		docker("rm", "-f", name)
		again := t.Name("again")
		start(again)
		if c, tok := login(su.AccessToken); c/100 != 2 || tok == "" {
			t.Fatalf("login after a restart: %d\n%s", c, t.Logs(again))
		}
		t.Logf("smoke test passed (ghostfolio %s: read-only boot, migrate + seed + server, health green, client served, first signup is ADMIN, access-token login, wrong token refused, kept across a restart; user %s)", t.Want, u)
	})
}
