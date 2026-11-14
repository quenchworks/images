package smoke

import (
	"regexp"
	"strings"
	"time"
)

func init() {
	// A language image: the interpreter is the entrypoint. Exercise it and pip on a read-only rootfs,
	// and check what the interpreter really imports for pip's vendored msgpack: Wolfi's pyc files are
	// unchecked-hash, so a patched .py beside stale bytecode would still run the old code.
	register("python", func(t *T) {
		if t.Want == "" {
			t.Fatalf("usage: qw smoke python <image-ref> <minor>")
		}
		minor := strings.Join(strings.SplitN(t.Want, ".", 3)[:2], ".")
		ro := []string{"--read-only", "--tmpfs", "/tmp"}
		py := func(args ...string) string { out, _ := t.RunFlags(ro, args...); return strings.TrimSpace(out) }
		t.Match(py("--version"), `^Python `+regexp.QuoteMeta(minor)+`\.`, "default entrypoint version")
		vv, _ := t.RunFlags(append(ro, "--entrypoint", "/usr/bin/python"+minor), "--version")
		t.Match(vv, `Python `+regexp.QuoteMeta(minor)+`\.`, "/usr/bin/python"+minor)
		if o := py("-c", `print("quench-" + "python")`); o != "quench-python" {
			t.Fatalf("script output %q", o)
		}
		t.Contains(py("-c", `import ssl, json, hashlib, sqlite3; print("stdlib ok", ssl.OPENSSL_VERSION)`), "stdlib ok", "stdlib imports")
		t.Contains(py("-m", "pip", "--version"), "pip ", "pip")
		if uid := py("-c", "import os; print(os.getuid())"); uid != "1001" {
			t.Fatalf("runs as uid %q, expected 1001", uid)
		}
		u := t.User("1001")
		if _, err := t.RunFlags(ro, "-c", `open("/should-fail","w")`); err == nil {
			t.Fatalf("rootfs was writable, expected read-only")
		}
		if v := py("-c", "from pip._vendor import msgpack; print(msgpack.__version__)"); v != "1.2.3" {
			t.Fatalf("vendored msgpack is %q, expected 1.2.3 (stale bytecode?)", v)
		}
		t.Contains(py("-c", `from pip._vendor import msgpack
from pip._vendor.cachecontrol.serialize import Serializer
payload = {"a": [1, 2, 3], "b": "x"}
assert msgpack.unpackb(msgpack.packb(payload)) == payload
print("msgpack round-trip ok")`), "msgpack round-trip ok", "pip's msgpack code path")
		seeded, _ := t.RunFlags([]string{"--read-only", "--tmpfs", "/tmp:rw,exec"}, "-c", `import venv, subprocess
venv.create("/tmp/v", with_pip=True)
print(subprocess.run(["/tmp/v/bin/python", "-c", "from pip._vendor import msgpack; print(msgpack.__version__)"], capture_output=True, text=True).stdout.strip())`)
		if strings.TrimSpace(seeded) != "1.2.3" {
			t.Fatalf("venv-seeded msgpack is %q, expected 1.2.3", seeded)
		}
		t.Logf("smoke test passed (Python %s, read-only rootfs, vendored msgpack 1.2.3 also in a fresh venv; user %s)", minor, u)
	})

	// php-fpm + nginx on a read-only rootfs with the config dir on a volume: the CLI installer runs
	// against a throwaway MariaDB, then the front page and the administrator login must render and
	// the CLI must report the version.
	register("joomla", func(t *T) {
		const base = "http://127.0.0.1:18097"
		u := t.User("1001")
		net_, db, app := t.Name("net"), t.Name("db"), t.Name("")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Docker("run", "-d", "--name", db, "--network", net_, "-e", "MARIADB_ROOT_PASSWORD=rootpw", "-e", "MARIADB_DATABASE=joomla",
			"-e", "MARIADB_USER=joomla", "-e", "MARIADB_PASSWORD=joomlapw", "mariadb:11")
		t.Cleanup(func() { docker("rm", "-f", db) })
		t.waitFor(60, time.Second, db, "mariadb never came up", func() bool {
			_, err := docker("exec", db, "mariadb", "-ujoomla", "-pjoomlapw", "-e", "select 1", "joomla")
			return err == nil
		})
		flags := []string{"--network", net_, "--read-only", "--tmpfs", "/tmp", "-e", "JOOMLA_CONFIG_DIR=/var/www/config", "-p", "127.0.0.1:18097:8080"}
		for _, d := range []string{"/var/www/html/images", "/var/www/config", "/var/www/html/tmp", "/var/www/html/cache",
			"/var/www/html/administrator/cache", "/var/www/html/administrator/logs"} {
			flags = append(flags, "--tmpfs", d+":mode=1777")
		}
		t.Start(app, flags)
		t.waitFor(30, time.Second, app, "nginx never answered", func() bool { c, _ := Get(base + "/"); return c != 0 })
		if out, err := docker("exec", app, "sh", "-c", `cd /var/www/html && php installation/joomla.php install --site-name=QuenchSmoke \
  --admin-user="Smoke Admin" --admin-username=smokeadmin --admin-password="Quench-Smoke-Pass-2026" --admin-email=smoke@example.com \
  --db-type=mysqli --db-host=`+db+` --db-user=joomla --db-pass=joomlapw --db-name=joomla --db-prefix=qw_ --db-encryption=0 --no-interaction`); err != nil {
			t.Fatalf("the CLI installer failed\n%s", out)
		}
		if _, err := docker("exec", app, "test", "-s", "/var/www/config/configuration.php"); err != nil {
			t.Fatalf("installer wrote no configuration.php on the config volume")
		}
		if _, err := docker("exec", app, "test", "!", "-e", "/var/www/html/configuration.php"); err != nil {
			t.Fatalf("configuration.php landed in the read-only docroot")
		}
		// the web installer stays out of reach once a site exists, and the CLI dir is never served
		for _, p := range []string{"installation/index.php", "cli/joomla.php"} {
			if c, _ := Get(base + "/" + p); c != 403 {
				t.Fatalf("/%s is reachable over HTTP (got %d)", p, c)
			}
		}
		c, page := Get(base + "/")
		if c != 200 || !strings.Contains(strings.ToLower(page), "quenchsmoke") {
			t.Fatalf("front page: HTTP %d, site name rendered: %v", c, strings.Contains(strings.ToLower(page), "quenchsmoke"))
		}
		c, adm := Get(base + "/administrator/")
		if c != 200 || !regexp.MustCompile(`mod-login-username|name="username"`).MatchString(adm) {
			t.Fatalf("administrator login: HTTP %d, no login form", c)
		}
		ver, _ := docker("exec", app, "sh", "-c", "cd /var/www/html && php cli/joomla.php --version")
		if t.Want != "" {
			t.Contains(ver, t.Want, "joomla CLI version")
		}
		if regexp.MustCompile(`PHP (Fatal|Parse) error|Uncaught`).MatchString(t.Logs(app)) {
			t.Fatalf("php logged a fatal error\n%s", t.Logs(app))
		}
		t.Logf("smoke test passed (joomla %s: installed into MariaDB, config on its volume, front page and administrator served; user %s)", t.Want, u)
	})
}
