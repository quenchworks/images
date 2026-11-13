package smoke

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// viaConnect fetches http://target/ through an HTTP proxy's CONNECT tunnel (curl -p).
func viaConnect(proxyAddr, target string) string {
	c, err := net.DialTimeout("tcp", proxyAddr, 5*time.Second)
	if err != nil {
		return err.Error()
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(10 * time.Second))
	fmt.Fprintf(c, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
	br := bufio.NewReader(c)
	r, err := http.ReadResponse(br, nil)
	if err != nil || r.StatusCode != 200 {
		return fmt.Sprint("CONNECT failed: ", err, r)
	}
	fmt.Fprintf(c, "GET / HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", target)
	r, err = http.ReadResponse(br, nil)
	if err != nil {
		return err.Error()
	}
	b, _ := io.ReadAll(r.Body)
	return string(b)
}

func init() {
	register("postgresql", func(t *T) {
		postgresLike(t)
		// postgresLike leaves its container running until cleanup; vector runs on a fresh one
		const pw = "qw-smoke-pass"
		name := t.Name("vec")
		t.Start(name, []string{"-e", "POSTGRES_PASSWORD=" + pw, "-e", "POSTGRES_DB=qwapp"})
		q := func(sql string) (string, error) {
			return docker("exec", "-e", "PGPASSWORD="+pw, name, "psql", "-h", "/var/run/postgresql", "-U", "postgres", "-d", "qwapp", "-v", "ON_ERROR_STOP=1", "-qtAc", sql)
		}
		t.waitFor(60, time.Second, name, "postgres did not become ready", func() bool { o, _ := q("select 1"); return o == "1" })
		out, err := q("create extension vector; create table items (id int, e vector(3)); insert into items values (1,'[1,2,3]'),(2,'[9,9,9]'); create index on items using hnsw (e vector_l2_ops); select id from items order by e <-> '[1,2,4]' limit 1;")
		if err != nil || lastLine(out) != "1" {
			t.Fatalf("pgvector nearest neighbour wrong: %s", out)
		}
		t.Logf("pgvector nearest-neighbour query ok")
	})
	register("prefect", func(t *T) {
		if t.Want == "" {
			t.Fatalf("usage: qw smoke prefect <image-ref> <version>")
		}
		u := t.User("1001")
		net_, name := t.Name("net"), t.Name("")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Start(name, []string{"--network", net_, "--read-only", "--tmpfs", "/tmp", "--tmpfs", "/var/lib/prefect:uid=1001,gid=1001", "-p", "127.0.0.1:14200:4200"})
		t.waitFor(120, time.Second, name, "server never became healthy", func() bool { _, b := Get("http://127.0.0.1:14200/api/health"); return strings.Contains(b, "true") })
		if _, v := Get("http://127.0.0.1:14200/api/admin/version"); v != `"`+t.Want+`"` {
			t.Fatalf("server version %s, expected %s", v, t.Want)
		}
		_, ui := Get("http://127.0.0.1:14200/")
		t.ContainsFold(ui, "<html", "UI served")
		out, _ := t.RunFlags([]string{"--network", net_, "--read-only", "--tmpfs", "/tmp", "--tmpfs", "/var/lib/prefect:uid=1001,gid=1001",
			"-e", "PREFECT_API_URL=http://" + name + ":4200/api", "--entrypoint", "/opt/prefect/venv/bin/python"}, "-c", `
from prefect import flow, task
@task
def add(a, b): return a + b
@flow(name="quench-smoke")
def smoke(): return add(40, 2)
print("result", smoke())
`)
		t.Contains(out, "result 42", "flow ran")
		_, runs := Post("http://127.0.0.1:14200/api/flow_runs/filter", "application/json", "{}")
		t.Contains(runs, `"state_type":"COMPLETED"`, "COMPLETED flow run on the server")
		t.Logf("smoke test passed (nonroot %s, prefect %s, flow run COMPLETED)", u, t.Want)
	})
	register("redis-exporter", func(t *T) {
		if v := t.RunAny("-version"); t.Want != "" {
			t.Contains(v, t.Want, "version")
		}
		_, m := t.httpUp([]string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-p", "127.0.0.1:9121:9121"}, "http://127.0.0.1:9121/metrics", 30)
		t.Contains(m, "redis_exporter_build_info", "/metrics")
		if t.Want != "" {
			t.Match(m, `redis_exporter_build_info\{[^}]*version="`+regexp.QuoteMeta(t.Want)+`"`, "build_info version")
		}
		t.Logf("smoke test passed (nonroot uid: %s, read-only rootfs)", t.User("1001"))
	})
	register("smokescreen", func(t *T) {
		u := t.User("1001")
		if v := t.RunAny("--version"); t.Want != "" {
			t.Contains(v, "v"+t.Want, "version")
		}
		net_, ok, no, name := t.Name("net"), t.Name("ok"), t.Name("no"), t.Name("")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Cleanup(func() { docker("rm", "-f", ok, no) })
		for _, n := range [][2]string{{ok, "backend-ok"}, {no, "backend-no"}} {
			t.Docker("run", "-d", "--name", n[0], "--network", net_, "--entrypoint", "/usr/bin/caddy", "ghcr.io/quenchworks/images/caddy:2.11.9", "respond", "--listen", ":8000", n[1])
		}
		ip := func(c string) string {
			return t.Docker("inspect", "-f", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", c)
		}
		okA, noA := ip(ok)+":8000", ip(no)+":8000"
		t.Start(name, []string{"--network", net_, "--read-only", "-p", "127.0.0.1:14750:4750"}, "--listen-port", "4750", "--allow-address", okA)
		const px = "http://127.0.0.1:14750"
		t.waitFor(30, time.Second, name, "allowed backend not reached over HTTP", func() bool { _, b := viaProxy(px, "http://"+okA+"/"); return b == "backend-ok" })
		if b := viaConnect("127.0.0.1:14750", okA); b != "backend-ok" {
			t.Fatalf("allowed backend not reached over CONNECT: %s", b)
		}
		if c, b := viaProxy(px, "http://"+noA+"/"); c == 200 || strings.Contains(b, "backend-no") {
			t.Fatalf("private backend was proxied (HTTP %d)", c)
		}
		logs := t.Logs(name)
		t.Match(logs, `(?i)deny|denied|not allowed|private`, "deny decision logged")
		if regexp.MustCompile(`panic|read-only file system`).MatchString(logs) {
			t.Fatalf("crashed or wrote to the root\n%s", logs)
		}
		t.Logf("smoke test passed: HTTP and CONNECT to an allowed address, private address denied, user %s", u)
	})
	register("solr", func(t *T) {
		const b = "http://127.0.0.1:8983/solr"
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/var/solr:rw,mode=1777", "-p", "127.0.0.1:8983:8983"})
		t.waitFor(60, 2*time.Second, name, "solr did not become ready", func() bool { _, s := Get(b + "/admin/info/system?wt=json"); return strings.Contains(s, `"lucene"`) })
		if c, r := Get(b + "/admin/cores?action=CREATE&name=gate&configSet=_default"); c != 200 {
			t.Fatalf("core create failed: %d %s\n%s", c, r, t.Logs(name))
		}
		if c, r := Post(b+"/gate/update?commit=true", "application/json", `[{"id":"1","title_s":"quench"}]`); c != 200 {
			t.Fatalf("index failed: %d %s", c, r)
		}
		_, q := Get(b + "/gate/select?q=id:1")
		t.Contains(q, `"quench"`, "query returned the indexed doc")
		t.Logf("smoke test passed (nonroot user: %s)", t.User("1001"))
	})
	register("weaviate", func(t *T) {
		name := t.Name("")
		t.Start(name, []string{"--tmpfs", "/tmp", "-e", "PERSISTENCE_DATA_PATH=/tmp/data", "-e", "AUTHENTICATION_ANONYMOUS_ACCESS_ENABLED=true",
			"-e", "QUERY_DEFAULTS_LIMIT=25", "-e", "CLUSTER_HOSTNAME=node1", "-p", "127.0.0.1:8080:8080"})
		t.WaitHTTP("http://127.0.0.1:8080/v1/.well-known/ready", 60, name)
		t.WaitHTTP("http://127.0.0.1:8080/v1/.well-known/live", 5, name)
		_, meta := Get("http://127.0.0.1:8080/v1/meta")
		t.Logf("meta: %s", meta)
		t.Logf("smoke test passed (nonroot user: %s)", t.User("1001"))
	})
	register("buildkit", func(t *T) {
		u := t.User("1001")
		v := t.RunEntry("/usr/bin/buildkitd", "--version")
		t.Match(v, ` v[01]\.`, "version stamped")
		if t.Want != "" {
			t.Match(v, ` v`+regexp.QuoteMeta(t.Want)+`( |$)`, "version")
		}
		name := t.Name("")
		t.Start(name, []string{"--security-opt", "seccomp=unconfined", "--security-opt", "apparmor=unconfined"}, "--oci-worker-no-process-sandbox")
		t.waitFor(60, time.Second, name, "buildkitd did not come up", func() bool { _, err := docker("exec", name, "buildctl", "debug", "workers"); return err == nil })
		ctx := "/home/nonroot/.local/tmp/ctx"
		t.Docker("exec", name, "sh", "-c", "mkdir -p "+ctx+" && cd "+ctx+` && echo quench > hello.txt && printf "FROM scratch\nCOPY hello.txt /hello.txt\n" > Dockerfile`)
		tar := filepath.Join(t.TempDir(), "out.tar")
		f, _ := os.Create(tar)
		c := exec.Command("docker", "exec", name, "buildctl", "build", "--frontend", "dockerfile.v0", "--local", "context="+ctx, "--local", "dockerfile="+ctx, "--output", "type=tar,dest=-")
		c.Stdout = f
		err := c.Run()
		f.Close()
		if err != nil {
			t.Fatalf("build failed: %v\n%s", err, t.Logs(name))
		}
		if got := tarFile(tar, "hello.txt"); got != "quench\n" {
			t.Fatalf("build output missing hello.txt (got %q)", got)
		}
		t.Logf("smoke test passed (%s, nonroot user: %s)", v, u)
	})
	register("ceph", func(t *T) {
		m := regexp.MustCompile(`(?m)^ceph version ([0-9.]+)`).FindStringSubmatch(t.Run("--version"))
		if m == nil || (t.Want != "" && m[1] != t.Want) {
			t.Fatalf("ceph version %v, expected %s", m, t.Want)
		}
		for _, b := range strings.Fields("ceph-mon ceph-mgr ceph-osd ceph-mds radosgw rbd ceph-volume ceph-exporter ceph-bluestore-tool python3") {
			if _, err := t.RunFlags([]string{"--entrypoint", "/bin/bash"}, "-c", "command -v "+b); err != nil {
				t.Fatalf("missing %s", b)
			}
		}
		name := t.Name("")
		t.Start(name, []string{"--entrypoint", "/bin/bash"}, "-c", `set -e
fsid=$(cat /proc/sys/kernel/random/uuid)
ip=$(hostname -i | cut -d" " -f1)
printf "[global]\nfsid = %s\nmon_initial_members = a\nmon_host = %s\nauth_allow_insecure_global_id_reclaim = false\n" "$fsid" "$ip" > /etc/ceph/ceph.conf
ceph-authtool --create-keyring /tmp/mon.keyring --gen-key -n mon. --cap mon "allow *"
ceph-authtool --create-keyring /etc/ceph/ceph.client.admin.keyring --gen-key -n client.admin \
  --cap mon "allow *" --cap osd "allow *" --cap mds "allow *" --cap mgr "allow *"
ceph-authtool /tmp/mon.keyring --import-keyring /etc/ceph/ceph.client.admin.keyring
chown ceph:ceph /tmp/mon.keyring
monmaptool --create --add a "$ip" --fsid "$fsid" /tmp/monmap
mkdir -p /var/lib/ceph/mon/ceph-a && chown -R ceph:ceph /var/lib/ceph/mon
ceph-mon --mkfs -i a --monmap /tmp/monmap --keyring /tmp/mon.keyring --setuser ceph --setgroup ceph
exec ceph-mon -f -i a --setuser ceph --setgroup ceph`)
		t.waitFor(40, 2*time.Second, name, "mon never formed quorum", func() bool {
			st, _ := docker("exec", name, "ceph", "-s", "--connect-timeout", "5")
			return strings.Contains(st, "mon: 1 daemons, quorum a")
		})
		t.Logf("smoke test passed (ceph %s, mon quorum)", m[1])
	})
}
