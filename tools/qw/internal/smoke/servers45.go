package smoke

import (
	"strings"
	"time"
)

func init() {
	register("prometheus", func(t *T) {
		const b = "http://127.0.0.1:9090"
		name, h := t.httpUp([]string{"--read-only", "--tmpfs", "/prometheus:rw,mode=1777", "--tmpfs", "/tmp:rw,mode=1777", "-p", "127.0.0.1:9090:9090"}, b+"/-/healthy", 30)
		t.ContainsFold(h, "Healthy", "/-/healthy body")
		t.WaitHTTP(b+"/-/ready", 30, name)
		t.waitFor(30, time.Second, name, "self-scrape 'up' metric not returned", func() bool {
			_, r := Get(b + "/api/v1/query?query=up")
			return strings.Contains(r, `"status":"success"`) && strings.Contains(r, `"__name__":"up"`)
		})
		_, ui := Get(b + "/")
		t.Match(ui, `(?i)<!doctype html|<html`, "embedded UI")
		t.Docker("exec", name, "/usr/bin/promtool", "--version")
		t.Logf("smoke test passed (nonroot uid: %s, read-only rootfs, healthy+ready+up+UI)", t.User("1001"))
	})
	register("coolify-app", func(t *T) {
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777,uid=9999,gid=9999", "--tmpfs", "/var/www/html/storage:rw,uid=9999,gid=9999",
			"--tmpfs", "/var/www/html/bootstrap/cache:rw,uid=9999,gid=9999", "-e", "SCHEDULER_ENABLED=false", "-e", "HORIZON_ENABLED=false",
			"-e", "NIGHTWATCH_ENABLED=false", "-p", "127.0.0.1:8080:8080"})
		var hc string
		t.waitFor(60, time.Second, name, "nginx+php-fpm /healthcheck did not become healthy", func() bool {
			if s := t.Docker("inspect", "-f", "{{.State.Status}}", name); s != "running" {
				t.Fatalf("container exited early\n%s", t.Logs(name))
			}
			var c int
			c, hc = Get("http://127.0.0.1:8080/healthcheck")
			return c == 200
		})
		t.Contains(hc, "OK", "/healthcheck payload")
		if out, err := docker("exec", "-w", "/var/www/html", name, "php", "artisan", "--version"); err != nil {
			t.Fatalf("php artisan --version failed (app did not boot): %s", out)
		}
		mods := t.Docker("exec", name, "php", "-m")
		for _, e := range strings.Fields("pdo_pgsql pgsql redis bcmath gd intl mbstring pcntl") {
			t.Match(mods, `(?mi)^`+e+`$`, "PHP extension "+e)
		}
		t.Docker("exec", name, "test", "-f", "/var/www/html/public/build/manifest.json")
		for _, bin := range strings.Fields("ssh git git-lfs psql php-fpm nginx supervisord") {
			if _, err := docker("exec", name, "sh", "-c", "command -v "+bin); err != nil {
				t.Fatalf("runtime tool %s missing", bin)
			}
		}
		t.Logf("smoke test passed (nonroot uid %s, nginx + php-fpm healthy, artisan boots, extensions, Vite assets, tooling)", t.User("9999"))
	})
	register("documentdb", func(t *T) {
		const pass, user = "quenchpw1", "default_user"
		const mongosh = "mongodb/mongodb-community-server:latest"
		net_, name := t.Name("net"), t.Name("")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Start(name, []string{"--network", net_, "-e", "DOCUMENTDB_PASSWORD=" + pass, "-p", "127.0.0.1:10260:10260"})
		t.WaitTCP(90, name, "127.0.0.1:10260")
		time.Sleep(3 * time.Second)
		uri := "mongodb://" + user + ":" + pass + "@" + name + ":10260/?tls=true&tlsAllowInvalidCertificates=true&authMechanism=SCRAM-SHA-256"
		sh := func(js string) string {
			out, _ := docker("run", "--rm", "--network", net_, mongosh, "mongosh", uri, "--quiet", "--eval", js)
			return out
		}
		t.waitFor(30, 2*time.Second, name, "ping failed", func() bool { return strings.TrimSpace(sh("db.runCommand({ping:1}).ok")) == "1" })
		out := sh(`const c = db.getSiblingDB("smoke").coll; c.insertOne({_id: 1, hello: "quench"}); print(c.findOne({_id: 1}).hello);`)
		t.Contains(out, "quench", "insert/find round trip")
		t.Logf("smoke test passed (mongo wire protocol through the gateway; user %s)", t.Docker("inspect", t.Image, "--format", "{{.Config.User}}"))
	})
}
