package smoke

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

func init() {
	register("redmine", func(t *T) {
		const pgImg = "ghcr.io/quenchworks/images/postgresql@sha256:4c0580fd2968b8d6e8fb008e0eb74b55c54464b041c235842e4d1cc37ed9a445"
		const u = "http://127.0.0.1:13000"
		net_, pg, app := t.Name("net"), t.Name("pg"), t.Name("")
		dbpw, adminpw, skb := "pg-"+hexRand(6), "Smoke-pw-"+hexRand(4)+"-x", hexRand(48)
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		// The postgresql image creates POSTGRES_DB only when it differs from POSTGRES_USER.
		t.Docker("run", "-d", "--name", pg, "--network", net_, "--network-alias", "pg", "-e", "POSTGRES_USER=redmine",
			"-e", "POSTGRES_PASSWORD="+dbpw, "-e", "POSTGRES_DB=redminedb", pgImg)
		t.Cleanup(func() { docker("rm", "-f", pg) })
		// pg_isready also passes on the temporary server the image starts to create the database;
		// three real queries in a row mean the final server is up.
		ok := 0
		t.waitFor(90, time.Second, pg, "postgres never ready", func() bool {
			if _, err := docker("exec", "-e", "PGPASSWORD="+dbpw, pg, "psql", "-h", "127.0.0.1", "-U", "redmine", "-d", "redminedb", "-tAc", "select 1"); err == nil {
				ok++
			} else {
				ok = 0
			}
			return ok == 3
		})
		env := []string{"--network", net_, "--read-only", "--tmpfs", "/tmp", "--tmpfs", "/data:uid=1001,gid=1001",
			"-e", "REDMINE_DB_HOST=pg", "-e", "REDMINE_DB_NAME=redminedb", "-e", "REDMINE_DB_USER=redmine",
			"-e", "REDMINE_DB_PASSWORD=" + dbpw, "-e", "SECRET_KEY_BASE=" + skb}
		if out, err := t.RunFlags(env, "rake", "db:migrate"); err != nil {
			t.Fatalf("db:migrate failed:\n%s", out)
		}
		t.Logf("  schema migrated")
		t.RunFlags(append(env, "-e", "REDMINE_LANG=en"), "rake", "redmine:load_default_data")
		out, _ := t.RunFlags(env, "runner", `Setting.rest_api_enabled = "1"; u = User.find_by_login("admin"); u.password = u.password_confirmation = ARGV[0]; u.must_change_passwd = false; u.save!; puts Redmine::VERSION.to_s`, adminpw)
		ver := strings.TrimSuffix(lastLine(out), ".stable") // Redmine::VERSION.to_s is "7.0.1.stable"
		t.Logf("redmine %s", ver)
		if t.Want != "" && ver != t.Want {
			t.Fatalf("expected %s", t.Want)
		}
		t.Start(app, append(env, "-p", "127.0.0.1:13000:3000"))
		t.waitCode(u+"/login", 90, time.Second, app, 200)
		_, page := Get(u + "/login")
		t.Contains(page, "Redmine", "login page")
		t.Logf("  /login renders")
		basic := func(pw string) map[string]string {
			return map[string]string{"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:"+pw))}
		}
		_, me := DoH("GET", u+"/users/current.json", "", basic(adminpw))
		t.Contains(me, `"login":"admin"`, "API accepts the admin")
		if c, _ := DoH("GET", u+"/users/current.json", "", basic("wrong-password")); c != 401 {
			t.Fatalf("a wrong password returned %d, expected 401", c)
		}
		t.Logf("  API accepts the admin password and refuses a wrong one")
		if m := regexp.MustCompile(`(?i)read-only file system|permission denied`).FindString(t.Logs(app)); m != "" {
			t.Fatalf("the app hit the read-only root (%s)", m)
		}
		t.Logf("smoke test passed (redmine %s, read-only rootfs, nonroot user: %s)", ver, t.User("1001"))
	})
}

func init() {
	register("umami", func(t *T) {
		const pgImg = "ghcr.io/quenchworks/images/postgresql@sha256:4c0580fd2968b8d6e8fb008e0eb74b55c54464b041c235842e4d1cc37ed9a445"
		const u = "http://127.0.0.1:13000"
		user := t.User("1001")
		ver := strings.TrimSpace(t.RunEntry("/usr/bin/node", "-p", `require("/app/package.json").version`))
		t.Logf("umami %s", ver)
		if t.Want != "" && ver != t.Want {
			t.Fatalf("expected %s", t.Want)
		}
		net_, pg, app, pw := t.Name("net"), t.Name("pg"), t.Name(""), "pg-"+hexRand(6)
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Docker("run", "-d", "--name", pg, "--network", net_, "--network-alias", "pg", "-e", "POSTGRES_PASSWORD="+pw, "-e", "POSTGRES_DB=umami", pgImg)
		t.Cleanup(func() { docker("rm", "-f", pg) })
		t.waitFor(60, time.Second, pg, "postgres never ready", func() bool {
			_, err := docker("exec", pg, "pg_isready", "-h", "127.0.0.1", "-U", "postgres", "-d", "umami")
			return err == nil
		})
		// start-docker.sh runs prisma migrate deploy through the schema engine first.
		t.Start(app, []string{"--network", net_, "-p", "127.0.0.1:13000:3000", "--read-only", "--tmpfs", "/tmp:uid=1001,gid=1001",
			"-e", "DATABASE_URL=postgresql://postgres:" + pw + "@pg:5432/umami", "-e", "APP_SECRET=smoke-" + hexRand(8)})
		t.waitFor(90, 2*time.Second, app, "/api/heartbeat never ok", func() bool {
			_, b := Get(u + "/api/heartbeat")
			return strings.Contains(b, `"ok":true`)
		})
		t.Logf("  migrated and up: /api/heartbeat ok")
		const js = "application/json"
		if c, _ := Post(u+"/api/auth/login", js, `{"username":"admin","password":"wrong"}`); c != 401 {
			t.Fatalf("a wrong password got HTTP %d", c)
		}
		_, login := Post(u+"/api/auth/login", js, `{"username":"admin","password":"umami"}`)
		var tok struct{ Token string }
		if json.Unmarshal([]byte(login), &tok); tok.Token == "" {
			t.Fatalf("no token: %s", login)
		}
		t.Logf("  admin login ok, wrong password refused")
		h := map[string]string{"Authorization": "Bearer " + tok.Token, "Content-Type": js}
		_, site := DoH("POST", u+"/api/websites", `{"name":"smoke","domain":"smoke.example"}`, h)
		var s struct{ ID string }
		if json.Unmarshal([]byte(site), &s); s.ID == "" {
			t.Fatalf("no website id: %s", site)
		}
		// /api/send drops requests that look like bots: send a browser user agent.
		DoH("POST", u+"/api/send", `{"type":"event","payload":{"website":"`+s.ID+`","hostname":"smoke.example","url":"/hello","language":"en-US","screen":"1920x1080"}}`,
			map[string]string{"Content-Type": js, "User-Agent": "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0 Safari/537.36"})
		now := time.Now().UnixMilli()
		var stats string
		t.waitFor(10, time.Second, app, "page view not counted", func() bool {
			_, stats = DoH("GET", fmt.Sprintf("%s/api/websites/%s/stats?startAt=%d&endAt=%d", u, s.ID, now-3600000, now+60000), "", h)
			var st struct{ Pageviews json.RawMessage }
			json.Unmarshal([]byte(stats), &st)
			// newer releases return {"value": n}, older ones a bare number
			var v struct{ Value float64 }
			var n float64
			if json.Unmarshal(st.Pageviews, &v) == nil && v.Value > 0 {
				return true
			}
			return json.Unmarshal(st.Pageviews, &n) == nil && n >= 1
		})
		t.Logf("  page view sent and counted: %s", stats)
		if m := regexp.MustCompile(`(?i)read-only file system|EROFS`).FindString(t.Logs(app)); m != "" {
			t.Fatalf("wrote to the root (%s)", m)
		}
		t.Logf("smoke test passed (umami %s: migrate on PostgreSQL, login, website, tracked page view in stats, read-only root; user %s)", ver, user)
	})
}
