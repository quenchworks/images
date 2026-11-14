package smoke

import (
	"encoding/json"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func init() {
	// php-fpm, nginx and the worker daemon on a read-only rootfs with the config dir on a volume:
	// autoinstall against a throwaway MariaDB, then nodeinfo names the version, the login form
	// renders, the swapped jQuery carries Migrate, code and config stay hidden, and the worker
	// daemon processes queued jobs.
	register("friendica", func(t *T) {
		const base = "http://127.0.0.1:18098"
		u := t.User("1001")
		net_, db, app := t.Name("net"), t.Name("db"), t.Name("")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Docker("run", "-d", "--name", db, "--network", net_, "-e", "MARIADB_ROOT_PASSWORD=rootpw", "-e", "MARIADB_DATABASE=friendica",
			"-e", "MARIADB_USER=friendica", "-e", "MARIADB_PASSWORD=friendicapw", "mariadb:11")
		t.Cleanup(func() { docker("rm", "-f", db) })
		sql := func(q string) (string, error) {
			return docker("exec", db, "mariadb", "-ufriendica", "-pfriendicapw", "-N", "-e", q, "friendica")
		}
		t.waitFor(60, time.Second, db, "mariadb never came up", func() bool { _, err := sql("select 1"); return err == nil })
		t.Start(app, []string{"--network", net_, "--read-only", "--tmpfs", "/tmp", "--tmpfs", "/var/friendica/config:mode=1777", "-p", "127.0.0.1:18098:8080"})
		t.waitFor(30, time.Second, app, "nginx never answered", func() bool { c, _ := Get(base + "/"); return c != 0 })
		if out, err := docker("exec", app, "php", "bin/console.php", "autoinstall", "--dbhost", db, "--dbuser", "friendica",
			"--dbpass", "friendicapw", "--dbdata", "friendica", "--admin", "admin@example.com", "--url", base); err != nil {
			t.Fatalf("autoinstall failed\n%s", out)
		}
		if _, err := docker("exec", app, "test", "-s", "/var/friendica/config/local.config.php"); err != nil {
			t.Fatalf("installer wrote no local.config.php on the config volume")
		}
		c, ni := Get(base + "/nodeinfo/2.0")
		var n struct {
			Software struct{ Name, Version string }
		}
		json.Unmarshal([]byte(ni), &n)
		if c != 200 || n.Software.Name != "friendica" || (t.Want != "" && !strings.Contains(n.Software.Version, t.Want)) {
			t.Fatalf("/nodeinfo/2.0: HTTP %d, software %+v", c, n.Software)
		}
		// the first requests after the install run the database update behind a maintenance page
		t.waitFor(30, 2*time.Second, app, "/.well-known/nodeinfo did not link nodeinfo 2.0", func() bool {
			_, wk := DoH("GET", base+"/.well-known/nodeinfo", "", map[string]string{"Accept": "application/json"})
			return strings.Contains(wk, "/nodeinfo/2.0")
		})
		r, err := httpc.Get(base + "/login")
		if err != nil {
			t.Fatalf("/login: %v", err)
		}
		lb, _ := io.ReadAll(r.Body)
		r.Body.Close()
		if r.StatusCode != 200 || !strings.Contains(string(lb), `name="password"`) {
			t.Fatalf("/login: HTTP %d, login form rendered: %v", r.StatusCode, strings.Contains(string(lb), `name="password"`))
		}
		_, js := Get(base + "/view/asset/jquery/dist/jquery.min.js")
		t.Contains(js, "jQuery v3.", "jQuery 3")
		t.Contains(js, "jQuery Migrate", "jQuery Migrate appended")
		for _, p := range []string{"config/local.config.php", "vendor/autoload.php", "bin/console.php", "src/App.php"} {
			if c, _ := Get(base + "/" + p); c != 403 {
				t.Fatalf("/%s is reachable over HTTP (got %d)", p, c)
			}
		}
		// the daemon runs in the foreground (no pidfile): prove it by the queued jobs it processed
		t.waitFor(40, 3*time.Second, app, "the worker daemon processed no queued job", func() bool {
			out, _ := sql("select count(*) from workerqueue where done = 1")
			n, _ := strconv.Atoi(strings.TrimSpace(out))
			return n > 0
		})
		if regexp.MustCompile(`PHP (Fatal|Parse) error|Uncaught`).MatchString(t.Logs(app)) {
			t.Fatalf("php logged a fatal error\n%s", t.Logs(app))
		}
		t.Logf("smoke test passed (friendica %s: installed, nodeinfo, login, jQuery Migrate, worker jobs done; user %s)", n.Software.Version, u)
	})

	// No database needed: on a read-only rootfs with the chart's writable carve-outs, nginx and
	// php-fpm serve the installer, the CLI boots, the swapped Twig is the fixed version and
	// renders, and the required PHP extensions are loaded.
	register("matomo", func(t *T) {
		const base = "http://127.0.0.1:18081"
		u := t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777,uid=1001,gid=1001", "--tmpfs", "/var/www/html/tmp:rw,uid=1001,gid=1001",
			"--tmpfs", "/var/www/html/config:rw,uid=1001,gid=1001", "-p", "127.0.0.1:18081:8080"})
		t.waitFor(60, time.Second, name, "the installer did not answer 200", func() bool {
			if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
				t.Fatalf("container exited early\n%s", t.Logs(name))
			}
			c, _ := Get(base + "/index.php")
			return c == 200
		})
		_, body := Get(base + "/index.php")
		t.ContainsFold(body, "matomo", "installer page")
		v := t.Docker("exec", "-w", "/var/www/html", name, "php", "console", "--version")
		if t.Want != "" {
			t.Contains(v, t.Want, "matomo console version")
		}
		// the recipe swaps the vendored Twig and drops its classmap rows: the version alone would
		// not catch broken autoloading, so render loops, escaping and filters too
		out, err := docker("exec", "-w", "/var/www/html", name, "php", "-r", `
require "vendor/autoload.php";
$v = \Twig\Environment::VERSION;
if (version_compare($v, "3.27.0", "<")) { fwrite(STDERR, "twig too old: $v\n"); exit(1); }
$l = new \Twig\Loader\ArrayLoader(["t" => "{% for i in [1,2] %}{{ i }}{% endfor %}|{{ \"<b>\"|escape }}|{{ \"a,b\"|split(\",\")|join(\"-\") }}"]);
$out = (new \Twig\Environment($l, ["cache" => false]))->render("t");
if ($out !== "12|&lt;b&gt;|a-b") { fwrite(STDERR, "twig render mismatch: $out\n"); exit(1); }
echo "twig $v ok";`)
		if err != nil {
			t.Fatalf("twig assertion failed\n%s", out)
		}
		mods := t.Docker("exec", name, "php", "-m")
		for _, ext := range []string{"pdo_mysql", "mysqli", "gd", "mbstring", "dom", "curl", "openssl", "zip", "gmp", "intl"} {
			t.Match(mods, `(?im)^`+ext+`$`, "PHP extension "+ext)
		}
		t.ContainsFold(mods, "opcache", "PHP extension opcache")
		t.Logf("smoke test passed (matomo %s: installer served read-only, CLI boots, %s, extensions present; user %s)", t.Want, out, u)
	})
}
