package smoke

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func hexRand(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func init() {
	register("linkerd-viz-tap", func(t *T) {
		d := t.TempDir()
		tlsDir := filepath.Join(d, "tls")
		os.Mkdir(tlsDir, 0o755)
		crt, key := certPair("tap-injector")
		os.WriteFile(filepath.Join(tlsDir, "tls.crt"), crt, 0o644)
		os.WriteFile(filepath.Join(tlsDir, "tls.key"), key, 0o644)
		b, _ := os.ReadFile(t.DeadKubeconfig())
		os.WriteFile(filepath.Join(d, "kubeconfig"), b, 0o644)
		name := t.Name("")
		t.Start(name, []string{"-p", "127.0.0.1:9995:9995", "-p", "127.0.0.1:8443:8443", "-v", tlsDir + ":/var/run/linkerd/tls:ro", "-v", d + "/kubeconfig:/cfg/kubeconfig:ro"},
			"injector", "-kubeconfig=/cfg/kubeconfig", "-tap-service-name=tap.linkerd-viz.serviceaccount.identity.linkerd.cluster.local")
		t.waitFor(30, time.Second, name, "admin server (:9995/ping) did not answer", func() bool { _, p := Get("http://127.0.0.1:9995/ping"); return strings.TrimSpace(p) == "pong" })
		var code int
		t.waitFor(15, time.Second, name, "webhook TLS server did not answer", func() bool { code, _, _ = getH(insecure, "https://127.0.0.1:8443/"); return code != 0 })
		if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
			t.Fatalf("process exited\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed (admin ping, webhook :8443 HTTP %d)", code)
	})
	register("mastodon", func(t *T) {
		const b = "http://127.0.0.1:13000"
		u := t.User("1001")
		net_, pg, rd, web, sk := t.Name("net"), t.Name("pg"), t.Name("redis"), t.Name("web"), t.Name("sk")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Cleanup(func() { docker("rm", "-f", pg, rd, web, sk) })
		t.Docker("run", "-d", "--name", pg, "--network", net_, "-e", "POSTGRES_USER=mastodon", "-e", "POSTGRES_PASSWORD=pw", "-e", "POSTGRES_DB=mastodon_production", "postgres:17")
		t.Docker("run", "-d", "--name", rd, "--network", net_, "redis:7")
		t.waitFor(60, time.Second, pg, "postgres never ready", func() bool {
			_, err := docker("exec", pg, "psql", "-U", "mastodon", "-d", "mastodon_production", "-c", "select 1")
			return err == nil
		})
		flags := []string{"--network", net_, "--read-only", "--tmpfs", "/tmp:mode=1777,exec", "--tmpfs", "/opt/mastodon/public/system:uid=1001,gid=1001"}
		for _, e := range []string{"LOCAL_DOMAIN=smoke.example.com", "DB_HOST=" + pg, "DB_USER=mastodon", "DB_PASS=pw", "DB_NAME=mastodon_production",
			"REDIS_HOST=" + rd, "SECRET_KEY_BASE=" + hexRand(64), "OTP_SECRET=" + hexRand(64),
			"ACTIVE_RECORD_ENCRYPTION_DETERMINISTIC_KEY=smokedeterministickeysmokedeterm",
			"ACTIVE_RECORD_ENCRYPTION_KEY_DERIVATION_SALT=smokesaltsmokesaltsmokesaltsmoke",
			"ACTIVE_RECORD_ENCRYPTION_PRIMARY_KEY=smokeprimarykeysmokeprimarykeysm"} {
			flags = append(flags, "-e", e)
		}
		if out, err := t.RunFlags(append(flags, "-e", "SAFETY_ASSURED=1"), "rails", "db:setup"); err != nil {
			t.Fatalf("rails db:setup failed:\n%s", out)
		}
		t.Start(web, append(flags, "-p", "127.0.0.1:13000:3000"), "web")
		t.Start(sk, flags, "sidekiq")
		h := map[string]string{"Host": "smoke.example.com", "X-Forwarded-Proto": "https"}
		get := func(p string) (int, string) {
			r, _ := http.NewRequest("GET", b+p, nil)
			r.Host = h["Host"]
			r.Header.Set("X-Forwarded-Proto", "https")
			resp, err := noRedirect.Do(r)
			if err != nil {
				return 0, err.Error()
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			return resp.StatusCode, string(body)
		}
		t.waitFor(90, time.Second, web, "puma never answered /health", func() bool { c, _ := get("/health"); return c == 200 })
		_, about := get("/about")
		t.Contains(about, "/packs/", "about page references the compiled packs")
		_, inst := get("/api/v2/instance")
		t.Contains(inst, `"domain":"smoke.example.com"`, "instance API")
		var iv struct{ Version string }
		json.Unmarshal([]byte(inst), &iv)
		if t.Want != "" && iv.Version != t.Want {
			t.Fatalf("expected %s, instance reports %s", t.Want, iv.Version)
		}
		t.waitFor(60, time.Second, sk, "sidekiq did not start", func() bool {
			return regexp.MustCompile(`Booted Rails|Starting processing`).MatchString(t.Logs(sk))
		})
		vips, _ := t.RunFlags(flags, "rails", "runner", `require "vips"; puts "vips #{Vips.version_string}"`)
		t.Match(vips, `(?m)^vips 8\.`, "libvips loads")
		t.Logf("smoke test passed (mastodon %s, uid %s, schema, web, sidekiq, packs, libvips)", iv.Version, u)
	})
	register("mongo-express", func(t *T) {
		mongo := envOr("MONGO_IMAGE", "ghcr.io/quenchworks/images/mongodb:8.0.32")
		v, err := t.TryRun("-p", `JSON.parse(require("fs").readFileSync("package.json")).version`)
		if err != nil || v == "" {
			v = t.RunEntry("/usr/bin/node", "-p", `JSON.parse(require("fs").readFileSync("/usr/lib/mongo-express/package.json")).version`)
		}
		if t.Want != "" && v != t.Want {
			t.Fatalf("mongo-express %s, expected %s", v, t.Want)
		}
		net_, db, name := t.Name("net"), t.Name("db"), t.Name("")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Cleanup(func() { docker("rm", "-f", db) })
		t.Docker("run", "-d", "--name", db, "--network", net_, "--network-alias", "mongo", "--read-only", "--tmpfs", "/data:rw,mode=1777,exec",
			"-e", "MONGO_INITDB_ROOT_USERNAME=root", "-e", "MONGO_INITDB_ROOT_PASSWORD=rootpw", mongo)
		t.waitFor(60, 2*time.Second, db, "mongodb never accepted the insert", func() bool {
			_, err := docker("exec", "-e", "HOME=/data/log", db, "mongosh", "--quiet", "-u", "root", "-p", "rootpw", "--eval", `db.getSiblingDB("quench").items.insertOne({probe: "quench-smoke-doc"})`)
			return err == nil
		})
		t.Start(name, []string{"--network", net_, "--read-only", "--tmpfs", "/tmp", "-p", "127.0.0.1:18081:8081",
			"-e", "ME_CONFIG_MONGODB_URL=mongodb://root:rootpw@mongo:27017/?authSource=admin", "-e", "ME_CONFIG_BASICAUTH_ENABLED=true",
			"-e", "ME_CONFIG_BASICAUTH_USERNAME=quench", "-e", "ME_CONFIG_BASICAUTH_PASSWORD=smoke-pass", "-e", "ME_CONFIG_SITE_SESSIONSECRET=smoke-session",
			"-e", "ME_CONFIG_SITE_COOKIESECRET=smoke-cookie"})
		t.WaitHTTP("http://127.0.0.1:18081/status", 60, name)
		if c, _ := Get("http://127.0.0.1:18081/"); c != 401 {
			t.Fatalf("expected 401 without credentials, got %d", c)
		}
		_, page := Get("http://quench:smoke-pass@127.0.0.1:18081/db/quench/items")
		t.Contains(page, "quench-smoke-doc", "document visible through the UI")
		t.Logf("smoke test passed (mongo-express %s: status, basic auth, document listed; user %s)", v, t.Docker("inspect", t.Image, "--format", "{{.Config.User}}"))
	})
	register("pgadmin", func(t *T) {
		const b = "http://127.0.0.1:15050"
		u := t.User("1001")
		if v := t.RunEntry("/opt/pgadmin/venv/bin/python", "-c", "import importlib.metadata as m; print(m.version('pgadmin4'))"); t.Want != "" && v != t.Want {
			t.Fatalf("version mismatch: %s", v)
		}
		net_, pg, name := t.Name("net"), t.Name("pg"), t.Name("")
		pw, apw := t.Name("pg"), t.Name("Quench")+"-Pw1"
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Cleanup(func() { docker("rm", "-f", pg) })
		t.Docker("run", "-d", "--name", pg, "--network", net_, "-e", "POSTGRES_PASSWORD="+pw, "ghcr.io/quenchworks/images/postgresql:18.6")
		t.waitFor(60, 2*time.Second, pg, "postgres did not become ready", func() bool {
			_, err := docker("exec", "-e", "PGPASSWORD="+pw, pg, "psql", "-h", "127.0.0.1", "-U", "postgres", "-tAc", "select 1")
			return err == nil
		})
		t.Start(name, []string{"--network", net_, "--read-only", "--tmpfs", "/tmp", "--tmpfs", "/var/lib/pgadmin:uid=1001,gid=1001,mode=0700", "-p", "127.0.0.1:15050:5050",
			"-e", "PGADMIN_SETUP_EMAIL=admin@example.com", "-e", "PGADMIN_SETUP_PASSWORD=" + apw})
		t.waitFor(120, time.Second, name, "pgadmin never answered /misc/ping", func() bool { _, p := Get(b + "/misc/ping"); return p == "PING" })
		jar, _ := cookiejar.New(nil)
		c := &http.Client{Timeout: 30 * time.Second, Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		_, _, login := getH(c, b+"/login")
		m := regexp.MustCompile(`"csrfToken": *"([^"]*)"`).FindStringSubmatch(login)
		if m == nil {
			t.Fatalf("no CSRF token on the login page")
		}
		tok := m[1]
		r, err := c.PostForm(b+"/authenticate/login", url.Values{"email": {"admin@example.com"}, "password": {apw}, "csrf_token": {tok}})
		if err != nil {
			t.Fatalf("login: %v", err)
		}
		r.Body.Close()
		if !strings.HasSuffix(r.Header.Get("Location"), "/browser/") {
			t.Fatalf("login went to '%s'", r.Header.Get("Location"))
		}
		req := func(method, path, body string) string {
			rq, _ := http.NewRequest(method, b+path, strings.NewReader(body))
			rq.Header.Set("X-pgA-CSRFToken", tok)
			rq.Header.Set("Content-Type", "application/json")
			resp, err := c.Do(rq)
			if err != nil {
				t.Fatalf("%s %s: %v", method, path, err)
			}
			defer resp.Body.Close()
			out, _ := io.ReadAll(resp.Body)
			return string(out)
		}
		out := req("POST", "/browser/server/obj/1/", `{"name":"smoke","host":"`+pg+`","port":5432,"db":"postgres","username":"postgres","password":"`+pw+`","sslmode":"prefer","connect_now":true}`)
		t.Match(out, `"connected": *true`, "server connected")
		sid := regexp.MustCompile(`^\{"node":\{"_id": *"?([0-9]+)`).FindStringSubmatch(out)
		if sid == nil {
			t.Fatalf("no server id in: %.200s", out)
		}
		t.Match(req("GET", "/browser/database/nodes/1/"+sid[1]+"/", ""), `"label": *"postgres"`, "databases listed")
		if m := regexp.MustCompile(`Traceback|CRITICAL|ERROR`).FindString(t.Logs(name)); m != "" {
			t.Fatalf("pgadmin logged errors\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed (uid %s, admin login, server connected through libpq, databases listed)", u)
	})
}
