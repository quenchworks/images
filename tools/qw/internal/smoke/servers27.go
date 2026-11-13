package smoke

import (
	"encoding/binary"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// dnsAsk sends one query over network ("udp" or "tcp") and returns the reply.
func dnsAsk(network, server, host string, qt dnsmessage.Type) (*dnsmessage.Message, error) {
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: 0x5158, RecursionDesired: true})
	b.StartQuestions()
	b.Question(dnsmessage.Question{Name: dnsmessage.MustNewName(host + "."), Type: qt, Class: dnsmessage.ClassINET})
	q, err := b.Finish()
	if err != nil {
		return nil, err
	}
	c, err := net.DialTimeout(network, server, 3*time.Second)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	var buf []byte
	if network == "tcp" { // TCP messages carry a two-byte length prefix
		c.Write(append(binary.BigEndian.AppendUint16(nil, uint16(len(q))), q...))
		var l [2]byte
		if _, err := io.ReadFull(c, l[:]); err != nil {
			return nil, err
		}
		buf = make([]byte, binary.BigEndian.Uint16(l[:]))
		if _, err := io.ReadFull(c, buf); err != nil {
			return nil, err
		}
	} else {
		c.Write(q)
		buf = make([]byte, 4096)
		n, err := c.Read(buf)
		if err != nil {
			return nil, err
		}
		buf = buf[:n]
	}
	var m dnsmessage.Message
	return &m, m.Unpack(buf)
}

func init() {
	register("authentik", func(t *T) {
		t.User("1001")
		v := t.RunEntry("/usr/bin/authentik-server", "--version")
		t.Match(v, `[0-9]{4}\.[0-9]`, "CalVer version")
		if t.Want != "" {
			t.Contains(v, t.Want, "version")
		}
		cfg := t.RunEntryAny("/usr/bin/ak", "dump_config")
		if strings.TrimSpace(cfg) == "" {
			cfg = t.RunEntry("/ak-root/.venv/bin/python", "-c", `import authentik; import django; print("authentik core import OK; django", django.get_version())`)
		}
		t.Match(cfg, `(?i)authentik|postgresql|redis|django|"[a-z_]+"`, "authentik core config")
		py := []string{"--user", "1001", "--entrypoint", "/ak-root/.venv/bin/python"}
		out, err := t.RunFlags(append(py, "-e", "AUTHENTIK_SECRET_KEY=smoketest", "-e", "DJANGO_SETTINGS_MODULE=authentik.root.settings"),
			"-c", `import django; django.setup(); print("django.setup OK")`)
		if err != nil || !strings.Contains(out, "django.setup OK") {
			t.Fatalf("django.setup() could not populate the app registry:\n%s", out)
		}
		if out, err := t.RunFlags(py, "-c", `import guardian, django_channels_postgres, django_dramatiq_postgres, django_postgres_cache; print("ok")`); err != nil {
			t.Fatalf("a uv-workspace-local package is not importable:\n%s", out)
		}
		t.Logf("smoke test passed (uid 1001, server version, Python core, django.setup, workspace packages; full boot in the chart kind gate)")
	})
	register("bind", func(t *T) {
		u := t.Docker("inspect", t.Image, "--format", "{{.Config.User}}")
		v := t.RunEntry("/usr/bin/named", "-v")
		if t.Want != "" && !strings.HasPrefix(v, "BIND "+t.Want+" ") {
			t.Fatalf("version %s, expected BIND %s", v, t.Want)
		}
		n1 := t.Name("default")
		t.Start(n1, nil)
		t.waitFor(30, time.Second, n1, "the default config did not reach running", func() bool {
			return regexp.MustCompile(`(?m)running$`).MatchString(t.Logs(n1))
		})
		w := t.TempDir()
		os.WriteFile(filepath.Join(w, "named.conf"), []byte(`options { directory "/tmp"; pid-file none; listen-on port 5353 { any; }; recursion no; allow-query { any; }; };
zone "quench.test" { type primary; file "/work/quench.test.zone"; };
`), 0o644)
		os.WriteFile(filepath.Join(w, "quench.test.zone"), []byte("$TTL 60\n@   IN SOA ns.quench.test. admin.quench.test. 1 60 60 600 60\n@   IN NS  ns.quench.test.\nns  IN A   192.0.2.1\nwww IN A   192.0.2.80\n"), 0o644)
		n2 := t.Name("zone")
		t.Docker("create", "--name", n2, "-p", "127.0.0.1:15353:5353/udp", "-p", "127.0.0.1:15353:5353/tcp", "--entrypoint", "/usr/bin/named", t.Image, "-g", "-c", "/work/named.conf")
		t.Cleanup(func() { docker("rm", "-f", n2) })
		t.Docker("cp", w+"/.", n2+":/work")
		t.Docker("start", n2)
		const srv = "127.0.0.1:15353"
		t.waitFor(30, time.Second, n2, "UDP A query never answered 192.0.2.80", func() bool {
			m, err := dnsAsk("udp", srv, "www.quench.test", dnsmessage.TypeA)
			if err != nil || len(m.Answers) == 0 {
				return false
			}
			a, ok := m.Answers[0].Body.(*dnsmessage.AResource)
			return ok && net.IP(a.A[:]).String() == "192.0.2.80"
		})
		m, err := dnsAsk("tcp", srv, "quench.test", dnsmessage.TypeSOA)
		if err != nil || len(m.Answers) == 0 {
			t.Fatalf("TCP SOA query failed: %v\n%s", err, t.Logs(n2))
		}
		soa, ok := m.Answers[0].Body.(*dnsmessage.SOAResource)
		if !ok || soa.NS.String() != "ns.quench.test." || soa.MBox.String() != "admin.quench.test." || soa.Serial != 1 {
			t.Fatalf("TCP SOA query got %v", m.Answers[0].Body)
		}
		t.Logf("smoke test passed (uid %s, default config runs, zone answers over UDP and TCP)", u)
	})
	register("caddy", func(t *T) {
		t.RunEntry("/usr/bin/caddy", "version")
		w := t.TempDir()
		os.WriteFile(filepath.Join(w, "Caddyfile"), []byte("{\n\tadmin :2019\n}\n:8080 {\n\trespond \"ok 200\" 200\n}\n"), 0o644)
		name, _ := t.httpUp([]string{"--read-only", "--tmpfs", "/tmp", "--tmpfs", "/data", "--tmpfs", "/config", "-v", w + "/Caddyfile:/etc/caddy/Caddyfile:ro",
			"-p", "127.0.0.1:8080:8080", "-p", "127.0.0.1:2019:2019"}, "http://127.0.0.1:8080/", 30)
		if c, _ := Get("http://127.0.0.1:2019/config/"); c != 200 {
			t.Fatalf("admin :2019/config/ returned %d\n%s", c, t.Logs(name))
		}
		t.Logf("smoke test passed (nonroot user: %s, read-only rootfs, :8080 + admin)", t.User("1001"))
	})
	register("camel-k", func(t *T) {
		u := t.User("1001")
		ro := []string{"--read-only", "--tmpfs", "/tmp:exec", "--tmpfs", "/etc/maven/m2:uid=1001"}
		if v, _ := t.RunFlags(ro, "version"); t.Want != "" {
			t.Contains(v, t.Want, "version")
		}
		sh := append(ro, "--entrypoint", "/bin/sh")
		m, _ := t.RunFlags(sh, "-c", "$MAVEN_CMD -version 2>/dev/null")
		t.Contains(m, "Apache Maven 3.9.", "Maven 3.9")
		t.Contains(m, "Java version: 17.", "Maven on JDK 17")
		mw, _ := t.RunFlags(sh, "-c", "mkdir /tmp/b && cd /tmp/b && cp --recursive /usr/share/maven/mvnw/. . && ./mvnw -version 2>/dev/null")
		t.Contains(mw, "Apache Maven 3.9.", "mvnw shim")
		if out, err := t.RunFlags([]string{"--entrypoint", "/bin/sh"}, "-c", "touch /etc/maven/m2/x"); err != nil {
			t.Fatalf("/etc/maven/m2 not writable: %s", out)
		}
		kc := t.DeadKubeconfig()
		out, _ := t.RunAnyFor(60*time.Second, append(ro, "-v", kc+":/etc/kubeconfig:ro", "-e", "KUBECONFIG=/etc/kubeconfig", "-e", "WATCH_NAMESPACE=",
			"-e", "NAMESPACE=camel-k", "-e", "POD_NAME=camel-k-operator", "-e", "OPERATOR_ID=camel-k"), "operator")
		t.Contains(out, "127.0.0.1:1", "operator dialed the configured API server")
		if m := regexp.MustCompile(`panic:|read-only file system`).FindString(out); m != "" {
			t.Fatalf("operator crashed or wrote to the root: %s", m)
		}
		t.Logf("smoke test passed (uid %s, Maven on JDK 17, operator dialed the API server)", u)
	})
	register("crossplane", func(t *T) {
		v := strings.TrimSpace(strings.ReplaceAll(t.RunAny("--version"), "\r", ""))
		t.Match(v, `^v[0-9]+\.[0-9]+\.[0-9]+`, "version stamped")
		help := t.RunAny("--help")
		for _, c := range []string{"core start", "core init", "rbac start"} {
			t.Contains(help, c, "--help")
		}
		start := t.RunAny("core", "start", "--help")
		for _, f := range []string{"--webhook-port", "--metrics-port", "--health-probe-port"} {
			t.Contains(start, f, "core start --help")
		}
		ini := t.RunAny("core", "init", "--help")
		t.Contains(ini, `--cr-ds-path="/crds"`, "core init CRDs path")
		t.Contains(ini, `--webhook-configurations-path="/webhookconfigurations"`, "core init webhook path")
		files := strings.Join(t.Files(""), "\n")
		crds := regexp.MustCompile(`(?m)^crds/.*\.yaml$`).FindAllString(files, -1)
		if len(crds) < 15 {
			t.Fatalf("expected >=15 core CRDs at /crds, got %d", len(crds))
		}
		t.Match(files, `(?m)^webhookconfigurations/.*\.yaml$`, "webhook configuration")
		for _, f := range []string{"pkg.crossplane.io_providers", "pkg.crossplane.io_functions", "apiextensions.crossplane.io_compositeresourcedefinitions"} {
			t.Match(files, `(?m)^crds/`+regexp.QuoteMeta(f)+`\.yaml$`, "core CRD "+f)
		}
		t.Logf("smoke test passed (version %s, %d core CRDs, nonroot user: %s)", v, len(crds), t.User("1001"))
	})
	register("cube", func(t *T) {
		u := t.User("1001")
		v := t.RunEntry("/usr/bin/node", "-p", `require("/usr/lib/cube/node_modules/@cubejs-backend/server/package.json").version`)
		if t.Want != "" && v != t.Want {
			t.Fatalf("cube %s, expected %s", v, t.Want)
		}
		w := t.TempDir()
		os.MkdirAll(filepath.Join(w, "model/cubes"), 0o755)
		os.WriteFile(filepath.Join(w, "model/cubes/orders.yml"), []byte("cubes:\n  - name: orders\n    sql_table: public.orders\n    measures:\n      - name: count\n        type: count\n"), 0o644)
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp", "-v", w + "/model:/cube/conf/model:ro", "-e", "CUBEJS_DB_TYPE=postgres", "-e", "CUBEJS_DB_HOST=127.0.0.1",
			"-e", "CUBEJS_DB_NAME=x", "-e", "CUBEJS_DB_USER=x", "-e", "CUBEJS_DB_PASS=x", "-e", "CUBEJS_API_SECRET=smoke-test-secret", "-e", "CUBEJS_PG_SQL_PORT=15432",
			"-e", "CUBEJS_SQL_USER=cube", "-e", "CUBEJS_SQL_PASSWORD=cube", "-p", "127.0.0.1:14000:4000", "-p", "127.0.0.1:15432:15432"})
		t.WaitHTTP("http://127.0.0.1:14000/livez", 90, name)
		t.WaitTCP(30, name, "127.0.0.1:15432")
		t.Logf("smoke test passed: /livez ok, SQL API listening (native addon), user %s", u)
	})
}
