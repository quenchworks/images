package smoke

import (
	"crypto/sha256"
	"regexp"
	"strings"
	"time"
)

func init() {
	register("etcd", func(t *T) {
		const ep = "http://127.0.0.1:2379"
		name := t.Name("")
		t.Start(name, []string{"-e", "ETCD_LISTEN_CLIENT_URLS=http://0.0.0.0:2379", "-e", "ETCD_ADVERTISE_CLIENT_URLS=" + ep})
		for i := 0; ; i++ {
			if out, _ := docker("exec", name, "etcdctl", "--endpoints="+ep, "endpoint", "health"); strings.Contains(out, "healthy") {
				break
			}
			if i == 30 {
				t.Fatalf("etcd did not become healthy\n%s", t.Logs(name))
			}
			time.Sleep(time.Second)
		}
		t.Docker("exec", name, "etcdctl", "--endpoints="+ep, "put", "qw", "hello")
		if v := t.Docker("exec", name, "etcdctl", "--endpoints="+ep, "get", "qw", "--print-value-only"); v != "hello" {
			t.Fatalf("put/get failed: %s", v)
		}
		t.Logf("%s", strings.SplitN(t.Docker("exec", name, "etcd", "--version"), "\n", 2)[0])
		t.Logf("smoke test passed (nonroot user: %s)", t.User("1001"))
	})
	register("gost", func(t *T) {
		u := t.User("1001")
		v := t.Run("-V")
		if t.Want != "" {
			t.Contains(v, t.Want, "version")
		}
		name := t.Name("")
		t.Start(name, []string{"--read-only", "-p", "127.0.0.1:18080:8080", "-p", "127.0.0.1:19000:9000"},
			"-L", "http://smoke:smoke-pass@:8080", "-api", ":18081", "-metrics", ":9000")
		const api = "http://127.0.0.1:18081/config"
		for i := 0; ; i++ {
			if _, out := viaProxy("http://smoke:smoke-pass@127.0.0.1:18080", api); strings.Contains(out, `"services"`) {
				break
			}
			if i == 30 {
				t.Fatalf("API not reachable through the proxy\n%s", t.Logs(name))
			}
			time.Sleep(time.Second)
		}
		if c, _ := viaProxy("http://127.0.0.1:18080", api); c != 407 {
			t.Fatalf("no credentials got HTTP %d", c)
		}
		_, m := Get("http://127.0.0.1:19000/metrics")
		t.Match(m, `gost_|go_goroutines`, "metrics")
		t.Logf("smoke test passed: proxy, auth refusal (407), API, metrics, user %s", u)
	})
	register("linkding", func(t *T) {
		u := t.User("1001")
		v := t.RunEntry("/bin/cat", "/etc/linkding/version.txt")
		if t.Want != "" && v != t.Want {
			t.Fatalf("linkding %s, expected %s", v, t.Want)
		}
		name := t.Name("")
		t.Docker("volume", "create", name)
		t.Cleanup(func() { docker("volume", "rm", name) })
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp", "-v", name + ":/etc/linkding/data", "-p", "127.0.0.1:19090:9090",
			"-e", "LD_SUPERUSER_NAME=smoke", "-e", "LD_SUPERUSER_PASSWORD=smoke-pass-123"})
		t.WaitHTTP("http://127.0.0.1:19090/health", 120, name)
		out := t.Docker("exec", name, "python", "manage.py", "shell", "-c",
			"from django.contrib.auth.models import User; from bookmarks.models import ApiToken; print('TOKEN', ApiToken.objects.create(user=User.objects.get(username='smoke'), name='smoke').key)")
		m := regexp.MustCompile(`(?m)^TOKEN (\S+)`).FindStringSubmatch(out)
		if m == nil {
			t.Fatalf("no API token: %s", out)
		}
		h := map[string]string{"Authorization": "Token " + m[1], "Content-Type": "application/json"}
		if c, b := DoH("POST", "http://127.0.0.1:19090/api/bookmarks/", `{"url":"https://quench-works.com/smoke","title":"quench smoke","tag_names":["smoke"]}`, h); c/100 != 2 {
			t.Fatalf("create bookmark: %d %s", c, b)
		}
		_, list := DoH("GET", "http://127.0.0.1:19090/api/bookmarks/?q=smoke", "", h)
		t.Contains(list, "quench-works.com/smoke", "bookmark listed")
		if regexp.MustCompile(`Traceback|read-only file system`).MatchString(t.Logs(name)) {
			t.Fatalf("errors in the log:\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed: superuser from env, /health, REST API create and search, read-only root, user %s", u)
	})
	register("mysql", func(t *T) {
		const pw = "qw-smoke-pass"
		name := t.Name("")
		t.Start(name, []string{"-e", "MYSQL_ROOT_PASSWORD=" + pw, "-e", "MYSQL_DATABASE=qwapp"})
		sock := "--socket=/run/mysqld/mysqld.sock"
		for i := 0; ; i++ {
			if out, _ := docker("exec", name, "mysqladmin", sock, "-uroot", "-p"+pw, "ping"); strings.Contains(out, "mysqld is alive") {
				break
			}
			if i == 90 {
				t.Fatalf("mysql did not become ready\n%s", t.Logs(name))
			}
			time.Sleep(time.Second)
		}
		out, _ := docker("exec", name, "mysql", sock, "-uroot", "-p"+pw, "qwapp", "-e",
			"create table t(id int primary key, v varchar(32)); insert into t values (1,'hello'); select v from t where id=1;")
		t.Contains(out, "hello", "DDL/DML")
		t.Logf("%s", t.Docker("exec", name, "mysqld", "--version"))
		t.Logf("smoke test passed (nonroot user: %s)", t.User("1001"))
	})
	register("skopeo", func(t *T) {
		const src = "ghcr.io/quenchworks/images/distribution:3.1.2"
		u := t.User("1001")
		v := t.Run("--version")
		if t.Want != "" {
			t.Contains(v, "version "+t.Want, "version")
		}
		net_, reg := t.Name("net"), t.Name("reg")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Cleanup(func() { docker("rm", "-f", reg) })
		t.Docker("run", "-d", "--name", reg, "--network", net_, "--tmpfs", "/var/lib/registry:uid=1001,gid=1001", src)
		flags := []string{"--network", net_, "--read-only", "--tmpfs", "/tmp", "--tmpfs", "/home/nonroot:uid=1001,gid=1001"}
		dst := "docker://" + reg + ":5000/smoke/distribution"
		for i := 0; ; i++ {
			out, err := t.RunFlags(flags, "copy", "--all", "--dest-tls-verify=false", "docker://"+src, dst+":v1")
			if err == nil {
				break
			}
			if i == 30 {
				t.Fatalf("copy failed: %s", out)
			}
			time.Sleep(2 * time.Second)
		}
		raw := func(args ...string) [32]byte {
			out, err := t.RunFlags(flags, args...)
			if err != nil {
				t.Fatalf("inspect: %s", out)
			}
			return sha256.Sum256([]byte(out))
		}
		if raw("inspect", "--raw", "docker://"+src) != raw("inspect", "--raw", "--tls-verify=false", dst+":v1") {
			t.Fatalf("copied index differs")
		}
		tags, _ := t.RunFlags(flags, "list-tags", "--tls-verify=false", dst)
		t.Contains(tags, `"v1"`, "tag listed")
		t.Logf("smoke test passed (uid %s, multi-arch copy byte-identical)", u)
	})
	register("spicedb", func(t *T) {
		t.Match(t.RunAny("version"), `(?i)v?[0-9]+\.[0-9]+\.[0-9]+`, "version stamped")
		u := t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"-p", "127.0.0.1:50051:50051"}, "serve-testing", "--grpc-addr", ":50051", "--readonly-grpc-enabled=false", "--http-enabled=false")
		t.WaitTCP(30, name, "127.0.0.1:50051")
		if s := t.Docker("inspect", "-f", "{{.State.Status}}", name); s != "running" {
			t.Fatalf("container exited\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed (gRPC API reachable on 50051; nonroot user: %s)", u)
	})
	register("tika", func(t *T) {
		u := t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"-p", "127.0.0.1:19998:9998"})
		ver := t.WaitHTTP("http://127.0.0.1:19998/version", 90, name)
		t.Logf("%s", ver)
		if t.Want != "" {
			t.Contains(ver, t.Want, "version")
		}
		doc := `<html><head><title>QuenchWorks smoke</title></head><body><p>extracted by tika</p></body></html>`
		_, txt := DoH("PUT", "http://127.0.0.1:19998/tika", doc, map[string]string{"Content-Type": "text/html", "Accept": "text/plain"})
		t.Contains(txt, "extracted by tika", "/tika text")
		_, meta := DoH("PUT", "http://127.0.0.1:19998/meta", doc, map[string]string{"Content-Type": "text/html", "Accept": "application/json"})
		t.Contains(meta, `"dc:title":"QuenchWorks smoke"`, "/meta title")
		t.Logf("smoke test passed (uid %s, text and metadata extracted)", u)
	})
}
