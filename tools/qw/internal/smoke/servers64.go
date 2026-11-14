package smoke

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func init() {
	// php-fpm + nginx on a read-only rootfs with files/ on a tmpfs and the database from
	// BACKDROP_DB_*: the web installer answers, the CLI installer runs against a throwaway MariaDB,
	// then the front page renders the site name and /user/login (a clean URL) the login form.
	register("backdrop", func(t *T) {
		const base = "http://127.0.0.1:18096"
		u := t.User("1001")
		net_, db, app := t.Name("net"), t.Name("db"), t.Name("")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Docker("run", "-d", "--name", db, "--network", net_, "-e", "MARIADB_ROOT_PASSWORD=rootpw", "-e", "MARIADB_DATABASE=backdrop",
			"-e", "MARIADB_USER=backdrop", "-e", "MARIADB_PASSWORD=backdroppw", "mariadb:11")
		t.Cleanup(func() { docker("rm", "-f", db) })
		t.waitFor(60, time.Second, db, "mariadb never came up", func() bool {
			_, err := docker("exec", db, "mariadb", "-ubackdrop", "-pbackdroppw", "-e", "select 1", "backdrop")
			return err == nil
		})
		t.Start(app, []string{"--network", net_, "--read-only", "--tmpfs", "/tmp", "--tmpfs", "/var/www/html/files:mode=1777",
			"-e", "BACKDROP_DB_HOST=" + db, "-e", "BACKDROP_DB_NAME=backdrop", "-e", "BACKDROP_DB_USER=backdrop",
			"-e", "BACKDROP_DB_PASSWORD=backdroppw", "-e", "BACKDROP_HASH_SALT=smoke-salt-0123456789", "-p", "127.0.0.1:18096:8080"})
		t.waitFor(30, time.Second, app, "the web installer did not answer HTTP 200", func() bool { c, _ := Get(base + "/core/install.php"); return c == 200 })
		out, err := docker("exec", app, "php", "core/scripts/install.sh", "--db-url=mysql://backdrop:backdroppw@"+db+"/backdrop",
			"--account-name=smokeadmin", "--account-pass=Quench-Smoke-Pass-2026", "--account-mail=smoke@example.com",
			"--site-name=QuenchSmoke", "--site-mail=smoke@example.com")
		if err != nil || !strings.Contains(out, "installed successfully") {
			t.Fatalf("the CLI installer failed\n%s", out)
		}
		if _, err := docker("exec", app, "test", "-s", "/var/www/html/files/config/active/system.core.json"); err != nil {
			t.Fatalf("no active config on the files volume")
		}
		// CLI scripts, config and includes are never served
		for _, p := range []string{"core/scripts/install.sh", "files/config/active/system.core.json", "core/includes/bootstrap.inc"} {
			if c, _ := Get(base + "/" + p); c != 403 {
				t.Fatalf("/%s is reachable over HTTP (got %d)", p, c)
			}
		}
		if c, page := Get(base + "/"); c != 200 || !strings.Contains(page, "QuenchSmoke") {
			t.Fatalf("front page: HTTP %d, site name rendered: %v", c, strings.Contains(page, "QuenchSmoke"))
		}
		if c, login := Get(base + "/user/login"); c != 200 || !strings.Contains(login, `name="pass"`) {
			t.Fatalf("/user/login (clean URL): HTTP %d, no login form", c)
		}
		ver, _ := docker("exec", app, "php", "-r", `$s = file_get_contents("core/includes/bootstrap.inc"); preg_match("/BACKDROP_VERSION., .([0-9.]+)/", $s, $m); echo $m[1];`)
		if t.Want != "" && strings.TrimSpace(ver) != t.Want {
			t.Fatalf("expected Backdrop %s, the tree says %s", t.Want, ver)
		}
		mods, _ := docker("exec", app, "php", "-m")
		for _, ext := range []string{"pdo_mysql", "gd", "mbstring", "curl", "zip", "intl", "apcu", "dom", "zlib"} {
			t.Match(mods, `(?im)^`+ext+`$`, "PHP extension "+ext)
		}
		if regexp.MustCompile(`PHP (Fatal|Parse) error|Uncaught`).MatchString(t.Logs(app)) {
			t.Fatalf("php logged a fatal error\n%s", t.Logs(app))
		}
		t.Logf("smoke test passed (backdrop %s: installed into MariaDB, front page and login served, extensions present; user %s)", strings.TrimSpace(ver), u)
	})

	// Tempo single-binary (target: all) on a read-only rootfs with /var/tempo and /tmp on tmpfs:
	// /ready and /metrics answer 200.
	register("tempo", func(t *T) {
		const base = "http://127.0.0.1:13200"
		u := t.User("1001")
		v := t.Run("--version")
		if t.Want != "" {
			t.Contains(v, t.Want, "tempo version")
		}
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "tempo.yaml"), []byte(`target: all
server:
  http_listen_port: 3200
  grpc_listen_port: 9095
distributor:
  receivers:
    otlp:
      protocols:
        grpc:
          endpoint: 0.0.0.0:4317
        http:
          endpoint: 0.0.0.0:4318
storage:
  trace:
    backend: local
    wal:
      path: /var/tempo/wal
    local:
      path: /var/tempo/blocks
usage_report:
  reporting_enabled: false
`), 0o644)
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/var/tempo:rw,uid=1001,gid=1001", "--tmpfs", "/tmp:rw,uid=1001,gid=1001",
			"-v", dir + "/tempo.yaml:/etc/tempo/tempo.yaml:ro", "-p", "127.0.0.1:13200:3200"}, "-config.file=/etc/tempo/tempo.yaml")
		// Tempo answers 503 until startup completes
		t.waitFor(60, time.Second, name, "tempo /ready did not become ready", func() bool { c, _ := Get(base + "/ready"); return c == 200 })
		if c, _ := Get(base + "/metrics"); c != 200 {
			t.Fatalf("/metrics returned %d\n%s", c, t.Logs(name))
		}
		t.Logf("smoke test passed (tempo %s: read-only rootfs, /ready and /metrics 200; user %s)", t.Want, u)
	})
}
