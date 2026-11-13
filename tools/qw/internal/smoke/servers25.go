package smoke

import (
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// mariadbUp waits until mariadb-admin ping answers in name.
func (t *T) mariadbUp(name, pw string, tries int) {
	t.waitFor(tries, time.Second, name, "mariadb did not become ready", func() bool {
		out, _ := docker("exec", name, "mariadb-admin", "--socket=/run/mysqld/mysqld.sock", "-uroot", "-p"+pw, "ping")
		return strings.Contains(out, "mysqld is alive")
	})
}

func init() {
	register("yourls", func(t *T) {
		const b = "http://127.0.0.1:8080"
		dbImg := envOr("YOURLS_TEST_DB_IMAGE", "ghcr.io/quenchworks/images/mariadb:11.8.9")
		net_, db, app := t.Name("net"), t.Name("db"), t.Name("")
		pw, adminPW := t.Name("dbpw"), t.Name("adminpw")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Cleanup(func() { docker("rm", "-f", db) })
		t.Docker("run", "-d", "--name", db, "--network", net_, "-e", "MARIADB_ROOT_PASSWORD="+pw, "-e", "MARIADB_DATABASE=yourls",
			"-e", "MARIADB_USER=yourls", "-e", "MARIADB_PASSWORD="+pw, dbImg)
		t.mariadbUp(db, pw, 120)
		t.Start(app, []string{"--network", net_, "-p", "127.0.0.1:8080:8080", "-e", "YOURLS_DB_HOST=" + db + ":3306", "-e", "YOURLS_DB_USER=yourls",
			"-e", "YOURLS_DB_PASS=" + pw, "-e", "YOURLS_SITE=" + b, "-e", "YOURLS_COOKIEKEY=" + t.Name("cookie"), "-e", "YOURLS_USER=admin", "-e", "YOURLS_PASS=" + adminPW})
		t.waitCode(b+"/admin/install.php", 30, time.Second, app, 200)
		t.Docker("exec", app, "yourls-install")
		t.Contains(t.Docker("exec", app, "yourls-install"), "already installed", "second install run is a no-op")
		_, out := Post(b+"/yourls-api.php", "application/x-www-form-urlencoded", url.Values{"url": {"https://quench-works.com/smoke"},
			"action": {"shorturl"}, "keyword": {"qwsmoke"}, "format": {"json"}, "username": {"admin"}, "password": {adminPW}}.Encode())
		t.Match(out, `"statusCode":200|"status":"success"`, "shorten")
		r, err := noRedirect.Get(b + "/qwsmoke")
		if err != nil {
			t.Fatalf("GET /qwsmoke: %v", err)
		}
		r.Body.Close()
		if r.StatusCode != 301 || r.Header.Get("Location") != "https://quench-works.com/smoke" {
			t.Fatalf("redirect wrong: %d %s", r.StatusCode, r.Header.Get("Location"))
		}
		_, bad := Post(b+"/yourls-api.php", "application/x-www-form-urlencoded", url.Values{"action": {"stats"}, "format": {"json"}, "username": {"admin"}, "password": {"wrong"}}.Encode())
		t.Match(bad, `"errorCode":"?403`, "bad password refused")
		t.Logf("smoke test passed (short link redirects, auth enforced; user %s)", t.Docker("inspect", t.Image, "--format", "{{.Config.User}}"))
	})
	register("cratedb", func(t *T) {
		const b = "http://127.0.0.1:14200"
		name := t.Name("")
		t.Start(name, []string{"-p", "127.0.0.1:14200:4200"}, "-Cpath.data=/data", "-Cnetwork.host=_site_", "-Cdiscovery.type=single-node", "-Cauth.host_based.enabled=false")
		up := func() {
			t.waitFor(90, 2*time.Second, name, "CrateDB never answered", func() bool { c, _ := Get(b + "/"); return c == 200 })
		}
		sql := func(stmt string) string { _, r := Post(b+"/_sql", "application/json", `{"stmt":"`+stmt+`"}`); return r }
		up()
		_, root := Get(b + "/")
		m := regexp.MustCompile(`"number" *: *"([^"]*)"`).FindStringSubmatch(root)
		if m == nil || (t.Want != "" && m[1] != t.Want) {
			t.Fatalf("version %v, expected %s", m, t.Want)
		}
		sql("CREATE TABLE smoke (id INT PRIMARY KEY, v TEXT)")
		sql("INSERT INTO smoke (id, v) VALUES (1, 'quench'), (2, 'works')")
		sql("REFRESH TABLE smoke")
		t.Contains(sql("SELECT count(*) FROM smoke"), `"rows":[[2]]`, "count")
		t.Docker("restart", name)
		up()
		t.waitFor(30, 2*time.Second, name, "rows lost across a restart", func() bool { return strings.Contains(sql("SELECT v FROM smoke WHERE id = 2"), `"works"`) })
		t.Logf("smoke test passed (version %s, nonroot user: %s, SQL write/read, restart)", m[1], t.User("1001"))
	})
	register("floci", func(t *T) {
		u := t.User("1001")
		t.RunEntryAny("/usr/lib/jvm/java-25-openjdk/bin/java", "-version")
		name := t.Name("")
		t.Start(name, []string{"-p", "127.0.0.1:0:4566"})
		base := "http://127.0.0.1:" + t.hostPort(name, "4566/tcp")
		t.waitCode(base+"/_floci/health", 60, 2*time.Second, name, 200)
		bucket := t.Name("bucket")
		if c, _ := Do("PUT", base+"/"+bucket, ""); c != 200 && c != 201 && c != 204 {
			t.Fatalf("S3 create bucket: HTTP %d\n%s", c, t.Logs(name))
		}
		_, list := Get(base + "/")
		t.Contains(list, bucket, "created bucket in list-buckets")
		t.Logf("smoke test passed (nonroot user: %s, health, S3 create+list)", u)
	})
	register("harbor-registry", func(t *T) {
		t.Contains(t.RunEntryAny("/usr/bin/registry", "--version"), "v2.8.3-patch-redis", "registry version")
		name, _ := t.httpUp([]string{"--read-only", "--tmpfs", "/tmp", "--tmpfs", "/storage:uid=1001,gid=1001", "-p", "127.0.0.1:5000:5000", "-p", "127.0.0.1:5001:5001"},
			"http://127.0.0.1:5000/v2/", 30)
		for _, u := range []string{"http://127.0.0.1:5000/", "http://127.0.0.1:5001/metrics"} {
			if c, _ := Get(u); c != 200 {
				t.Fatalf("%s returned %d\n%s", u, c, t.Logs(name))
			}
		}
		t.Logf("smoke test passed (nonroot user: %s, read-only rootfs, /v2/ + / + metrics)", t.User("1001"))
	})
	register("mariadb", func(t *T) {
		const pw = "qw-smoke-pass"
		sock := "--socket=/run/mysqld/mysqld.sock"
		name := t.Name("")
		t.Start(name, []string{"-e", "MARIADB_ROOT_PASSWORD=" + pw, "-e", "MARIADB_DATABASE=qwapp"})
		t.mariadbUp(name, pw, 60)
		out, _ := docker("exec", name, "mariadb", sock, "-uroot", "-p"+pw, "qwapp", "-e",
			"create table t(id int primary key, v varchar(32)); insert into t values (1,'hello'); select v from t where id=1;")
		t.Contains(out, "hello", "DDL/DML")
		u := t.User("1001")
		// the MariaDB Operator runs the image as uid 999 with conf.d and extra args
		conf := t.TempDir()
		os.WriteFile(filepath.Join(conf, "quench.cnf"), []byte("[mariadbd]\nmax_connections=123\n"), 0o644)
		op := t.Name("op")
		t.Start(op, []string{"--user", "999:999", "--tmpfs", "/var/lib/mysql:uid=999,gid=999", "-e", "MARIADB_ROOT_PASSWORD=" + pw, "-v", conf + ":/etc/mysql/conf.d:ro"},
			"mariadbd", "--max-allowed-packet=33554432")
		t.mariadbUp(op, pw, 60)
		if got := t.Docker("exec", op, "mariadb", sock, "-uroot", "-p"+pw, "-N", "-e", "SELECT @@max_connections, @@max_allowed_packet"); got != "123\t33554432" {
			t.Fatalf("conf.d or args were not applied: %q", got)
		}
		t.Logf("%s", t.Docker("exec", name, "mariadbd", "--version"))
		t.Logf("smoke test passed (nonroot user: %s; uid 999 with conf.d and args)", u)
	})
	register("maven", func(t *T) {
		if t.Want == "" {
			t.Fatalf("usage: qw smoke maven <image-ref> <jdkmajor>")
		}
		ro := []string{"--read-only", "--tmpfs", "/tmp"}
		// stdout only: the JVM prints warnings on stderr
		b, err := exec.Command("docker", append(append([]string{"run", "--rm"}, ro...), t.Image, "-version")...).Output()
		if err != nil {
			t.Fatalf("mvn -version: %v", err)
		}
		c := string(b)
		t.Contains(c, "Apache Maven 3.9.", "maven version")
		t.Contains(c, "Java version: "+t.Want+".", "JDK version")
		jv, _ := t.RunFlags(append(ro, "--entrypoint", "/usr/bin/java"), "-version")
		t.Match(jv, `openjdk version "`+regexp.QuoteMeta(t.Want)+`(\.|")`, "bundled JDK")
		jh, _ := t.RunFlags(append(ro, "--entrypoint", "/usr/bin/printenv"), "JAVA_HOME")
		if jh != "/usr/lib/jvm/java-"+t.Want+"-openjdk" {
			t.Fatalf("JAVA_HOME is '%s'", jh)
		}
		t.User("1001")
		t.Contains(t.Docker("inspect", t.Image, "--format", "{{json .Config.Entrypoint}}"), "/usr/bin/mvn", "entrypoint")
		t.Logf("smoke test passed (Maven 3.9 on OpenJDK %s, read-only rootfs)", t.Want)
	})
}
