package smoke

import (
	"crypto/sha256"
	"encoding/asn1"
	"encoding/base64"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// postgresLike starts a Postgres-protocol image and runs DDL/DML on the database it creates.
func postgresLike(t *T) {
	const pw = "qw-smoke-pass"
	name := t.Name("")
	t.Start(name, []string{"-e", "POSTGRES_PASSWORD=" + pw, "-e", "POSTGRES_DB=qwapp"})
	psql := func(args ...string) (string, error) {
		return docker(append([]string{"exec", "-e", "PGPASSWORD=" + pw, name, "psql", "-h", "/var/run/postgresql", "-U", "postgres", "-d", "qwapp"}, args...)...)
	}
	for i := 0; ; i++ {
		if out, _ := psql("-qtAc", "select 1"); out == "1" {
			break
		}
		if i == 60 {
			t.Fatalf("postgres did not become ready\n%s", t.Logs(name))
		}
		time.Sleep(time.Second)
	}
	out, err := psql("-v", "ON_ERROR_STOP=1", "-c", "create table t(id int primary key, v text); insert into t values (1,'hello'); select v from t where id=1;")
	if err != nil || !strings.Contains(out, "hello") {
		t.Fatalf("DDL/DML failed: %s\n%s", out, t.Logs(name))
	}
	t.Logf("%s", t.Docker("exec", name, "postgres", "--version"))
	t.Logf("smoke test passed (nonroot user: %s)", t.User("1001"))
}

func init() {
	register("postgresql-15", postgresLike)
	register("nessie", func(t *T) {
		u := t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:exec", "-e", "NESSIE_VERSION_STORE_TYPE=ROCKSDB",
			"-e", "NESSIE_VERSION_STORE_PERSIST_ROCKS_DATABASE_PATH=/tmp/rocksdb", "-p", "127.0.0.1:19120:19120", "-p", "127.0.0.1:19000:9000"})
		for i := 0; ; i++ {
			if _, b := Get("http://127.0.0.1:19000/q/health/ready"); regexp.MustCompile(`"status": *"UP"`).MatchString(b) {
				break
			}
			if i == 90 {
				t.Fatalf("nessie never became ready\n%s", t.Logs(name))
			}
			time.Sleep(time.Second)
		}
		api := "http://127.0.0.1:19120/api/v2"
		_, cfg := Get(api + "/config")
		t.Match(cfg, `"defaultBranch" *: *"main"`, "config")
		if t.Want != "" {
			t.Contains(t.Logs(name), t.Want, "version in the startup log")
		}
		_, tree := Get(api + "/trees/main")
		m := regexp.MustCompile(`"hash" *: *"([0-9a-f]+)"`).FindStringSubmatch(tree)
		if m == nil {
			t.Fatalf("no hash for main: %s", tree)
		}
		if c, b := DoH("POST", api+"/trees?name=quench-smoke&type=BRANCH", `{"type":"BRANCH","name":"main","hash":"`+m[1]+`"}`, map[string]string{"Content-Type": "application/json"}); c/100 != 2 {
			t.Fatalf("create branch: %d %s", c, b)
		}
		_, trees := Get(api + "/trees")
		t.Match(trees, `"name" *: *"quench-smoke"`, "branch listed")
		if m := regexp.MustCompile(` ERROR |Exception`).FindString(t.Logs(name)); m != "" {
			t.Fatalf("nessie logged errors:\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed (uid %s, branch created and listed)", u)
	})
	register("opencost", func(t *T) {
		if t.Want == "" {
			t.Fatalf("usage: qw smoke opencost <image-ref> <version>")
		}
		u := t.User("1001")
		cid := t.Docker("create", t.Image)
		t.Cleanup(func() { docker("rm", "-f", cid) })
		dir := t.TempDir()
		for _, f := range []string{"default.json", "aws.json", "azure.json", "gcp.json"} {
			t.Docker("cp", cid+":/var/configs/"+f, filepath.Join(dir, f))
			if st, err := os.Stat(filepath.Join(dir, f)); err != nil || st.Size() == 0 {
				t.Fatalf("/var/configs/%s missing or empty", f)
			}
		}
		name, _ := t.httpUp([]string{"-p", "19003:9003"}, "http://127.0.0.1:19003/healthz", 60)
		t.Contains(t.Logs(name), t.Want, "version reported at startup")
		t.Logf("smoke test passed (nonroot user: %s, /healthz on 9003, version %s)", u, t.Want)
	})
	register("selenium-server", func(t *T) {
		u := t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp", "--tmpfs", "/home/nonroot:uid=1001,gid=1001", "-p", "127.0.0.1:14444:4444"}, "hub", "--session-request-timeout", "5")
		st := t.WaitHTTP("http://127.0.0.1:14444/status", 90, name)
		t.Contains(st, `"message"`, "/status")
		if t.Want != "" {
			t.Contains(t.Logs(name), "Started Selenium Hub "+t.Want, "hub version")
		}
		j := map[string]string{"Content-Type": "application/json; charset=utf-8"}
		_, gql := DoH("POST", "http://127.0.0.1:14444/graphql", `{"query":"{ grid { totalSlots sessionCount version } }"}`, j)
		t.Contains(gql, `"totalSlots"`, "graphql")
		c, body := DoH("POST", "http://127.0.0.1:14444/session", `{"capabilities":{"alwaysMatch":{"browserName":"firefox"}}}`, j)
		if c != 500 || !strings.Contains(body, "session not created") {
			t.Fatalf("POST /session: expected 500 session not created, got %d: %.600s", c, body)
		}
		t.Logf("smoke test passed (uid %s, queue answered a session request)", u)
	})
	register("terragrunt", func(t *T) {
		u := t.User("1001")
		if v := t.Run("--version"); t.Want != "" {
			t.Contains(v, "v"+t.Want, "version")
		}
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "terragrunt.hcl"), []byte("inputs = {\n  greeting = \"hello from terragrunt\"\n}\n"), 0o666)
		os.WriteFile(filepath.Join(dir, "main.tf"), []byte("variable \"greeting\" { type = string }\noutput \"message\" { value = upper(var.greeting) }\n"), 0o666)
		out, err := t.RunFlags([]string{"-v", dir + ":/work", "-e", "TG_NON_INTERACTIVE=true", "-e", "TERRAGRUNT_NON_INTERACTIVE=true"}, "apply", "-auto-approve")
		if err != nil {
			t.Fatalf("terragrunt apply failed:\n%s", out)
		}
		t.Contains(out, "Apply complete", "apply")
		msg, _ := docker("run", "--rm", "-v", dir+":/work", t.Image, "output", "-raw", "message")
		if !strings.HasSuffix(msg, "HELLO FROM TERRAGRUNT") {
			t.Fatalf("output was '%s'", msg)
		}
		t.Match(t.RunEntry("tofu", "version"), `(?i)opentofu|tofu`, "tofu present")
		t.Logf("smoke test passed (apply + output through tofu; user %s)", u)
	})
	register("timestamp-authority", func(t *T) {
		u := t.User("1001")
		t.Match(t.RunEntry("/usr/bin/timestamp-server", "version"), `GitVersion: *v`+regexp.QuoteMeta(t.Want), "version")
		name := t.Name("")
		t.Start(name, []string{"-p", "127.0.0.1:13000:3000"}, "--disable-ntp-monitoring")
		chain := t.WaitHTTP("http://127.0.0.1:13000/api/v1/timestamp/certchain", 30, name)
		t.Contains(chain, "BEGIN CERTIFICATE", "certificate chain")
		h := sha256.Sum256([]byte("quench"))
		c, reply := DoH("POST", "http://127.0.0.1:13000/api/v1/timestamp",
			`{"artifactHash":"`+base64.StdEncoding.EncodeToString(h[:])+`","hashAlgorithm":"sha256","certificates":true}`, map[string]string{"Content-Type": "application/json"})
		if c != 200 && c != 201 {
			t.Fatalf("timestamp not issued: %d\n%s", c, t.Logs(name))
		}
		// RFC 3161 TimeStampResp: SEQUENCE { PKIStatusInfo SEQUENCE { status INTEGER, ... }, token }
		var resp struct {
			Status asn1.RawValue
			Token  asn1.RawValue `asn1:"optional"`
		}
		var st struct{ Status int }
		if _, err := asn1.Unmarshal([]byte(reply), &resp); err != nil {
			t.Fatalf("reply is not a TimeStampResp: %v", err)
		}
		if _, err := asn1.Unmarshal(resp.Status.FullBytes, &st); err != nil || st.Status > 1 {
			t.Fatalf("reply not granted (status %d, %v)", st.Status, err)
		}
		t.Logf("smoke test passed (cert chain, RFC 3161 timestamp granted; nonroot user: %s)", u)
	})
	register("blackbox-exporter", func(t *T) {
		t.Run("--version")
		b := "http://127.0.0.1:9115"
		t.httpUp([]string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-p", "127.0.0.1:9115:9115"}, b+"/-/healthy", 30)
		_, m := Get(b + "/metrics")
		t.Contains(m, "blackbox_exporter_build_info", "/metrics")
		_, p := Get(b + "/probe?target=http://127.0.0.1:9115/metrics&module=http_2xx")
		t.Contains(p, "probe_success", "/probe")
		t.Logf("smoke test passed (nonroot uid: %s, read-only rootfs, healthy + /metrics + /probe)", t.User("1001"))
	})
	register("elasticsearch", func(t *T) {
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/conf:rw,mode=1777", "--tmpfs", "/data:rw,mode=1777",
			"--tmpfs", "/var/log/elasticsearch:rw,mode=1777", "--tmpfs", "/tmp:rw,mode=1777", "-p", "127.0.0.1:9200:9200"})
		for i := 0; ; i++ {
			if _, b := Get("http://127.0.0.1:9200/"); strings.Contains(b, `"cluster_name"`) {
				break
			}
			if i == 80 {
				t.Fatalf("elasticsearch did not become ready\n%s", t.Logs(name))
			}
			time.Sleep(3 * time.Second)
		}
		_, h := Get("http://127.0.0.1:9200/_cluster/health")
		t.Match(h, `"status":"(green|yellow)"`, "cluster health")
		if c, b := DoH("PUT", "http://127.0.0.1:9200/gate/_doc/1?refresh=true", `{"k":"quench"}`, map[string]string{"Content-Type": "application/json"}); c/100 != 2 {
			t.Fatalf("index: %d %s", c, b)
		}
		_, d := Get("http://127.0.0.1:9200/gate/_doc/1")
		t.Contains(d, `"quench"`, "get")
		t.Logf("smoke test passed (nonroot user: %s)", t.User("1001"))
	})
	register("fluentd", func(t *T) {
		v := lastLine(t.RunAny("--version"))
		if t.Want != "" && v != "fluentd "+t.Want {
			t.Fatalf("reported %s, expected fluentd %s", v, t.Want)
		}
		name := t.Name("")
		t.Start(name, []string{"-p", "127.0.0.1:19880:9880", "--read-only", "--tmpfs", "/tmp:mode=0777"})
		t.WaitLog(name, "fluentd worker is now running", 60)
		mark := t.Name("mark")
		for i := 0; ; i++ {
			if c, _ := Post("http://127.0.0.1:19880/smoke.test", "application/x-www-form-urlencoded", url.Values{"json": {`{"msg":"` + mark + `"}`}}.Encode()); c/100 == 2 {
				break
			}
			if i == 20 {
				t.Fatalf("HTTP input never accepted\n%s", t.Logs(name))
			}
			time.Sleep(time.Second)
		}
		re := regexp.MustCompile(`smoke\.test.*` + regexp.QuoteMeta(mark))
		for i := 0; !re.MatchString(t.Logs(name)); i++ {
			if i == 20 {
				t.Fatalf("event never reached stdout\n%s", t.Logs(name))
			}
			time.Sleep(time.Second)
		}
		t.Logf("smoke test passed (%s, nonroot user: %s, HTTP input to stdout)", v, t.User("1001"))
	})
	register("frr", func(t *T) {
		conf := t.TempDir()
		daemons := strings.Replace(t.RunEntry("/bin/cat", "/etc/frr/daemons"), "\nbgpd=no", "\nbgpd=yes", 1)
		os.WriteFile(filepath.Join(conf, "daemons"), []byte(daemons+"\n"), 0o644)
		os.WriteFile(filepath.Join(conf, "frr.conf"), []byte("frr defaults traditional\nhostname quench-smoke\nrouter bgp 64512\n bgp router-id 10.255.0.1\n no bgp ebgp-requires-policy\n neighbor 10.255.0.2 remote-as 64513\n"), 0o644)
		os.WriteFile(filepath.Join(conf, "vtysh.conf"), []byte("service integrated-vtysh-config\n"), 0o644)
		name := t.Name("")
		t.Start(name, []string{"--cap-add", "NET_ADMIN", "--cap-add", "NET_RAW", "--cap-add", "SYS_ADMIN", "-v", conf + ":/etc/frr"})
		var out string
		for i := 0; ; i++ {
			if out, _ = docker("exec", name, "vtysh", "-c", "show bgp summary"); strings.Contains(out, "local AS number 64512") {
				break
			}
			if i == 30 {
				t.Fatalf("bgpd never answered: %s\n%s", out, t.Logs(name))
			}
			time.Sleep(time.Second)
		}
		t.Contains(out, "10.255.0.2", "peer in the summary")
		m := regexp.MustCompile(`(?m)^FRRouting ([0-9.]+)`).FindStringSubmatch(t.Docker("exec", name, "vtysh", "-c", "show version"))
		if m == nil || (t.Want != "" && m[1] != t.Want) {
			t.Fatalf("version %v, expected %s", m, t.Want)
		}
		t.Contains(t.Docker("exec", name, "vtysh", "-c", "show interface eth0"), "Interface eth0", "zebra sees eth0")
		t.Logf("smoke test passed (bgpd AS 64512, peer configured, FRRouting %s)", m[1])
	})
}
