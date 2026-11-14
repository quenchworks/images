package smoke

import (
	"net/http"
	"net/url"
	"strings"
	"time"
)

func init() {
	// GoAlert on PostgreSQL: the server migrates the schema, add-user creates an administrator, the
	// administrator signs in through the basic provider and GraphQL answers as that user.
	register("goalert", func(t *T) {
		const pgImg = "ghcr.io/quenchworks/images/postgresql@sha256:4c0580fd2968b8d6e8fb008e0eb74b55c54464b041c235842e4d1cc37ed9a445"
		const base = "http://127.0.0.1:18081"
		u := t.User("1001")
		t.Contains(t.RunAny("version"), "v"+t.Want, "goalert version")
		net_, pg, app, pw := t.Name("net"), t.Name("pg"), t.Name(""), t.Name("pgpw")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		// The postgresql image creates POSTGRES_DB only when it differs from the superuser name.
		t.Docker("run", "-d", "--name", pg, "--network", net_, "--network-alias", "pg", "-e", "POSTGRES_PASSWORD="+pw, "-e", "POSTGRES_DB=goalert", pgImg)
		t.Cleanup(func() { docker("rm", "-f", pg) })
		t.waitFor(60, time.Second, pg, "postgres never ready", func() bool {
			_, err := docker("exec", pg, "pg_isready", "-h", "127.0.0.1", "-U", "postgres", "-d", "goalert")
			return err == nil
		})
		db := "postgres://postgres:" + pw + "@pg:5432/goalert?sslmode=disable"
		t.Start(app, []string{"--network", net_, "--read-only", "-p", "127.0.0.1:18081:8081",
			"-e", "GOALERT_DB_URL=" + db, "-e", "GOALERT_LISTEN=0.0.0.0:8081", "-e", "GOALERT_PUBLIC_URL=" + base})
		t.waitFor(90, 2*time.Second, app, "/health never ok", func() bool {
			c, _ := Get(base + "/health")
			return c == 200
		})
		t.Docker("run", "--rm", "--network", net_, t.Image, "add-user", "--db-url", db, "--user", "smoke", "--pass", "Quench-smoke-2026", "--admin")

		// sign in: the provider answers with a redirect and the session cookie
		nc := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		req, _ := http.NewRequest("POST", base+"/api/v2/identity/providers/basic", strings.NewReader(url.Values{"username": {"smoke"}, "password": {"Quench-smoke-2026"}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Referer", base+"/")
		resp, err := nc.Do(req)
		if err != nil {
			t.Fatalf("login: %v", err)
		}
		resp.Body.Close()
		var session string
		for _, c := range resp.Cookies() {
			if c.Name == "goalert_session.2" {
				session = c.Value
			}
		}
		if session == "" {
			t.Fatalf("login gave no session cookie (HTTP %d, Location %q)", resp.StatusCode, resp.Header.Get("Location"))
		}
		h := map[string]string{"Content-Type": "application/json", "Cookie": "goalert_session.2=" + session}
		c, b := DoH("POST", base+"/api/graphql", `{"query":"{ user { name role } }"}`, h)
		if c != 200 || !strings.Contains(b, `"name":"smoke"`) || !strings.Contains(b, `"role":"admin"`) {
			t.Fatalf("GraphQL as the administrator: %d %s", c, b)
		}
		_, ui := Get(base + "/")
		t.ContainsFold(ui, "<html", "web UI")
		t.Logf("smoke test passed (goalert %s: migrated PostgreSQL, admin signed in, GraphQL answered as admin, UI served; user %s)", t.Want, u)
	})
}
