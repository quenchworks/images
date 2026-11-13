package smoke

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// cql runs one CQL statement over native protocol v4 (no auth, consistency ONE) and returns the
// raw RESULT body. The caller searches it for the values it expects; rows are not decoded.
func cql(addr string, stmts ...string) ([]byte, error) {
	c, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(30 * time.Second))
	str := func(b *bytes.Buffer, s string) { binary.Write(b, binary.BigEndian, uint16(len(s))); b.WriteString(s) }
	frame := func(op byte, body []byte) ([]byte, error) {
		h := []byte{4, 0, 0, 0, op, 0, 0, 0, 0}
		binary.BigEndian.PutUint32(h[5:], uint32(len(body)))
		if _, err := c.Write(append(h, body...)); err != nil {
			return nil, err
		}
		rh := make([]byte, 9)
		if _, err := io.ReadFull(c, rh); err != nil {
			return nil, err
		}
		rb := make([]byte, binary.BigEndian.Uint32(rh[5:]))
		if _, err := io.ReadFull(c, rb); err != nil {
			return nil, err
		}
		if rh[4] == 0 {
			return nil, fmt.Errorf("cql error: %q", rb)
		}
		return rb, nil
	}
	var s bytes.Buffer
	binary.Write(&s, binary.BigEndian, uint16(1))
	str(&s, "CQL_VERSION")
	str(&s, "3.0.0")
	if _, err := frame(1, s.Bytes()); err != nil {
		return nil, err
	}
	var last []byte
	for _, q := range stmts {
		var b bytes.Buffer
		binary.Write(&b, binary.BigEndian, uint32(len(q)))
		b.WriteString(q)
		b.Write([]byte{0, 1, 0}) // consistency ONE, no flags
		if last, err = frame(7, b.Bytes()); err != nil {
			return nil, err
		}
	}
	return last, nil
}

func init() {
	register("cassandra", func(t *T) {
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/conf:rw,mode=1777", "--tmpfs", "/var/lib/cassandra:rw,mode=1777,exec",
			"--tmpfs", "/var/log/cassandra:rw,mode=1777", "-e", "MAX_HEAP_SIZE=512M", "-e", "HEAP_NEWSIZE=128M", "-p", "127.0.0.1:19042:9042"})
		un := regexp.MustCompile(`(?m)^UN .*`)
		var st string
		t.waitFor(60, 3*time.Second, name, "cassandra did not reach UN", func() bool {
			st, _ = docker("exec", name, "/opt/cassandra/bin/nodetool", "status")
			return un.MatchString(st)
		})
		t.Logf("node up: %s", un.FindString(st))
		// The runtime image has no python for cqlsh: speak the native protocol from Go.
		var res []byte
		var err error
		t.waitFor(30, 2*time.Second, name, "CQL roundtrip failed", func() bool {
			res, err = cql("127.0.0.1:19042",
				"CREATE KEYSPACE IF NOT EXISTS smoke WITH replication = {'class':'SimpleStrategy','replication_factor':1}",
				"CREATE TABLE IF NOT EXISTS smoke.t (k text PRIMARY KEY, v text)",
				"INSERT INTO smoke.t (k,v) VALUES ('quench','works')",
				"SELECT v FROM smoke.t WHERE k='quench'")
			return err == nil
		})
		if !bytes.Contains(res, []byte("works")) {
			t.Fatalf("roundtrip mismatch: %q", res)
		}
		t.Logf("CQL roundtrip OK: works")
		t.Logf("smoke test passed (nonroot user: %s)", t.User("1001"))
	})
	register("pgdog", func(t *T) {
		const pgImg = "ghcr.io/quenchworks/images/postgresql:18.6"
		user := t.User("1001")
		ver := strings.TrimSpace(t.Run("--version"))
		t.Logf("pgdog: %s", ver)
		t.WantVersionV(ver)
		net_, pg, dog := t.Name("net"), t.Name("pg"), t.Name("")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Docker("run", "-d", "--name", pg, "--network", net_, "-e", "POSTGRES_PASSWORD=quench", "-e", "POSTGRES_DB=app", pgImg)
		t.Cleanup(func() { docker("rm", "-f", pg) })
		t.waitFor(60, time.Second, pg, "postgres did not come up", func() bool {
			_, err := docker("exec", "-e", "PGPASSWORD=quench", pg, "psql", "-h", "/var/run/postgresql", "-U", "postgres", "-d", "app", "-qtAc", "select 1")
			return err == nil
		})
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "pgdog.toml"), []byte("[general]\nhost = \"0.0.0.0\"\nport = 6432\n[[databases]]\nname = \"app\"\nhost = \""+pg+"\"\nport = 5432\n[admin]\npassword = \"quench-admin\"\n"), 0o644)
		os.WriteFile(filepath.Join(dir, "users.toml"), []byte("[[users]]\nname = \"postgres\"\ndatabase = \"app\"\npassword = \"quench\"\n"), 0o644)
		t.Docker("create", "--name", dog, "--network", net_, t.Image)
		t.Cleanup(func() { docker("rm", "-f", dog) })
		for _, f := range []string{"pgdog.toml", "users.toml"} {
			t.Docker("cp", filepath.Join(dir, f), dog+":/etc/pgdog/"+f)
		}
		t.Docker("start", dog)
		// psql comes from the PostgreSQL container, pointed at PgDog.
		q := func(pw, u, db, sql string) (string, error) {
			out, err := docker("exec", "-e", "PGPASSWORD="+pw, pg, "psql", "-h", dog, "-p", "6432", "-U", u, "-d", db, "-qtAc", sql)
			return strings.TrimSpace(out), err
		}
		t.waitFor(30, time.Second, dog, "query through pgdog failed", func() bool { out, _ := q("quench", "postgres", "app", "select 40 + 2"); return out == "42" })
		t.Logf("query through pgdog: 42")
		if out, err := q("quench", "postgres", "app", "create table t (n int); insert into t values (1), (2), (3)"); err != nil {
			t.Fatalf("write through pgdog: %s", out)
		}
		if out, _ := q("quench", "postgres", "app", "select sum(n) from t"); out != "6" {
			t.Fatalf("write/read through pgdog failed: %s", out)
		}
		pools, _ := q("quench-admin", "admin", "admin", "SHOW POOLS")
		t.Contains(pools, "app", "admin SHOW POOLS")
		if _, err := q("wrong", "postgres", "app", "select 1"); err == nil {
			t.Fatalf("a wrong password was accepted")
		}
		t.Logf("smoke test passed (%s, nonroot user: %s)", ver, user)
	})
	register("phpmyadmin", func(t *T) {
		user := t.User("1001")
		name, body := t.httpUp([]string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777,uid=1001,gid=1001", "-p", "127.0.0.1:8097:8080"}, "http://127.0.0.1:8097/", 60)
		// The rendered login form proves Twig, the autoloader and config.inc.php all work.
		t.Contains(body, `name="pma_username"`, "phpMyAdmin login form")
		t.ContainsFold(body, "phpmyadmin", "body")
		if strings.Contains(strings.ToLower(body), "blowfish_secret") {
			t.Fatalf("login page complains about blowfish_secret")
		}
		t.Logf("login form rendered (Twig + autoloader + config.inc.php all live)")
		t.Logf("php: %s", strings.SplitN(t.Docker("exec", name, "php", "--version"), "\n", 2)[0])
		mods := t.Docker("exec", name, "php", "-m")
		for _, e := range strings.Fields("mysqli mysqlnd mbstring iconv openssl sodium zip bz2 gd curl ctype fileinfo xml dom simplexml json hash pcre session zlib") {
			t.Match(mods, `(?mi)^`+e+`$`, "PHP extension "+e)
		}
		t.ContainsFold(mods, "opcache", "OPcache")
		t.Docker("exec", name, "grep", "-q", "public const VERSION = '", "/var/www/html/libraries/classes/Version.php")
		// The remediated twig/twig (>= 3.27.0), not the 3.11.3 the tarball vendors.
		t.Docker("exec", name, "php", "-r", `$i=json_decode(file_get_contents("/var/www/html/vendor/composer/installed.json"),true);
foreach($i["packages"] as $p){ if($p["name"]==="twig/twig"){ echo "twig ",$p["version"],"\n";
exit(version_compare(ltrim($p["version"],"v"),"3.27.0","<") ? 1 : 0); } } exit(1);`)
		t.Docker("exec", name, "grep", "-q", "__proto__", "/var/www/html/js/vendor/js.cookie.js")
		// Front-end build metadata would only add phantom scanner findings.
		for _, f := range []string{"yarn.lock", "package.json"} {
			if _, err := docker("exec", name, "test", "-e", "/var/www/html/"+f); err == nil {
				t.Fatalf("front-end build metadata %s leaked into the image", f)
			}
		}
		for _, bin := range strings.Fields("php-fpm nginx supervisord") {
			if _, err := docker("exec", name, "sh", "-c", "command -v "+bin); err != nil {
				t.Fatalf("runtime tool %s missing", bin)
			}
		}
		t.Logf("smoke test passed (nonroot uid %s, read-only rootfs; nginx :8080 + php-fpm :9000 serve the phpMyAdmin login form)", user)
	})
}
