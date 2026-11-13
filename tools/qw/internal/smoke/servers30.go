package smoke

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func init() {
	register("zabbix-server", func(t *T) {
		const pgImg = "ghcr.io/quenchworks/images/postgresql:18.6"
		v := strings.SplitN(t.RunEntryAny("/usr/bin/zabbix_server", "-V"), "\n", 2)[0]
		t.Contains(v, "zabbix_server (Zabbix)", "zabbix_server -V")
		if t.Want != "" {
			t.Contains(v, "(Zabbix) "+t.Want, "version")
		}
		net_, pg, zs, vol := t.Name("net"), t.Name("pg"), t.Name(""), t.Name("conf")
		pw := t.Name("pw")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Cleanup(func() { docker("rm", "-f", pg) })
		t.Cleanup(func() { docker("volume", "rm", vol) })
		t.Docker("run", "-d", "--name", pg, "--network", net_, "-e", "POSTGRES_PASSWORD="+pw, "-e", "POSTGRES_DB=zabbix", pgImg)
		psql := []string{"exec", "-i", "-e", "PGPASSWORD=" + pw, pg, "psql", "-h", "/var/run/postgresql", "-U", "postgres", "-d", "zabbix"}
		t.waitFor(60, time.Second, pg, "postgres did not become ready", func() bool { o, _ := docker(append(psql, "-qtAc", "select 1")...); return o == "1" })
		dir := t.TempDir()
		cid := t.Docker("create", t.Image)
		t.Cleanup(func() { docker("rm", "-f", cid) })
		t.Docker("cp", cid+":/usr/share/zabbix/database/postgresql/.", dir)
		for _, f := range []string{"schema", "images", "data"} {
			b, err := os.ReadFile(filepath.Join(dir, f+".sql"))
			if err != nil {
				t.Fatalf("%s.sql: %v", f, err)
			}
			if out, err := runHostIn(string(b), "docker", append(psql, "-q", "-v", "ON_ERROR_STOP=1")...); err != nil {
				t.Fatalf("loading %s.sql: %s", f, out)
			}
		}
		t.Docker("volume", "create", vol)
		t.Docker("run", "--rm", "--user", "0", "-v", vol+":/c", "--entrypoint", "/bin/sh", pgImg, "-c",
			"printf 'DBHost="+pg+"\\nDBUser=postgres\\nDBPassword="+pw+"\\n' >/c/db.conf && chmod 644 /c/db.conf")
		t.Start(zs, []string{"--network", net_, "--read-only", "--tmpfs", "/tmp:uid=1001,gid=1001", "-v", vol + ":/etc/zabbix/zabbix_server.d:ro"})
		var logs string
		t.waitFor(60, time.Second, zs, "server processes never started", func() bool {
			if t.Docker("inspect", "-f", "{{.State.Running}}", zs) != "true" {
				t.Fatalf("server exited\n%s", t.Logs(zs))
			}
			logs = t.Logs(zs)
			return strings.Contains(logs, "server #0 started [main process]") && strings.Contains(logs, "started [trapper #1]")
		})
		if regexp.MustCompile(`(?i)database is down|cannot connect`).MatchString(logs) {
			t.Fatalf("database errors\n%s", logs)
		}
		t.Logf("smoke test passed (%s; schema loaded, server and trapper started on PostgreSQL 18)", v)
	})
	register("ansible", func(t *T) {
		t.User("1001")
		core := `core +[0-9]+\.[0-9]+\.[0-9]+`
		t.Match(t.RunEntry("/usr/bin/ansible", "--version"), core, "ansible --version")
		t.Match(t.RunEntry("/usr/bin/ansible-playbook", "--version"), core, "ansible-playbook --version")
		t.Match(t.RunEntry("/usr/bin/ansible-galaxy", "--version"), `[0-9]+\.[0-9]+\.[0-9]+`, "ansible-galaxy --version")
		if t.Want != "" {
			if cv := t.RunEntry("/opt/ansible/venv/bin/python", "-c", "import importlib.metadata as m; print(m.version('ansible'))"); cv != t.Want {
				t.Fatalf("community version %s, expected %s", cv, t.Want)
			}
		}
		out, _ := t.RunFlags([]string{"--read-only", "--tmpfs", "/tmp", "-e", "ANSIBLE_LOCAL_TEMP=/tmp/.ansible-tmp", "-e", "ANSIBLE_REMOTE_TEMP=/tmp/.ansible-tmp",
			"-e", "HOME=/tmp", "--entrypoint", "/usr/bin/ansible"}, "-m", "ping", "-c", "local", "localhost")
		t.Contains(out, "SUCCESS", "ping SUCCESS")
		t.Contains(out, `"ping": "pong"`, "ping pong")
		t.Logf("smoke test passed (CLIs report versions, ping localhost SUCCESS/pong, uid 1001)")
	})
	register("envoy-gateway", func(t *T) {
		name := t.Name("")
		t.Start(name, []string{"-p", "127.0.0.1:19002:19002"}, "envoy", "shutdown-manager")
		t.WaitHTTP("http://127.0.0.1:19002/healthz", 30, name)
		t.Contains(t.Logs(name), "starting shutdown manager", "shutdown manager log")
		docker("rm", "-f", name)
		out, code := t.RunAnyFor(120*time.Second, nil, "server")
		if code == 0 || code == 124 {
			t.Fatalf("server should fail fast without an API server (exit %d):\n%s", code, out)
		}
		t.Contains(out, "Start runners", "server reached its runners")
		vo := t.Run("version")
		field := func(k string) string {
			m := regexp.MustCompile(`(?m)^` + k + `: (\S+)`).FindStringSubmatch(vo)
			if m == nil {
				return ""
			}
			return m[1]
		}
		eg, px, gw := field("ENVOY_GATEWAY_VERSION"), field("ENVOY_PROXY_VERSION"), field("GATEWAYAPI_VERSION")
		if eg == "" || eg == "v0.0.0" || strings.Contains(eg, "dev") || (t.Want != "" && eg != "v"+t.Want) {
			t.Fatalf("envoy-gateway version %q, expected v%s", eg, t.Want)
		}
		t.Match(px, `^distroless-v[0-9]+\.[0-9]+\.[0-9]+(@sha256(:[0-9a-f]{64})?)?$`, "ENVOY_PROXY_VERSION")
		t.Match(gw, `^v[0-9]+\.[0-9]+\.[0-9]+$`, "GATEWAYAPI_VERSION")
		t.Logf("smoke test passed (envoy-gateway %s, data plane %s, gateway-api %s)", eg, px, gw)
	})
	register("jdk", func(t *T) {
		if t.Want == "" {
			t.Fatalf("usage: qw smoke jdk <image-ref> <major>")
		}
		major := strings.SplitN(t.Want, ".", 2)[0]
		ro := []string{"--read-only", "--tmpfs", "/tmp"}
		v, _ := t.RunFlags(ro, "-version")
		t.Match(v, `openjdk version "`+major+`(\.|")`, "java -version")
		jc, _ := t.RunFlags(append(ro, "--entrypoint", "/usr/bin/javac"), "-version")
		t.Match(jc, `^javac `+major+`(\.| )`, "javac -version")
		props, _ := t.RunFlags(append(ro, "--entrypoint", "/usr/bin/java"), "-XshowSettings:properties", "-version")
		t.Contains(props, "java.home = /usr/lib/jvm/java-"+major+"-openjdk\n", "java.home")
		c := exec.Command("docker", append(append([]string{"run", "--rm", "-i"}, ro...), "--entrypoint", "/usr/bin/jshell", t.Image,
			"-J-Djava.util.prefs.userRoot=/tmp", "-J-Djava.util.prefs.systemRoot=/tmp", "--feedback", "silent", "-")...)
		c.Stdin = strings.NewReader("System.out.print(\"quench-jdk-ok\");\n/exit\n")
		out, _ := c.Output() // stdout only: jshell warns on stderr
		if string(out) != "quench-jdk-ok" {
			t.Fatalf("jshell compile+run failed: got %q", out)
		}
		t.User("1001")
		t.Contains(t.Docker("inspect", t.Image, "--format", "{{json .Config.Entrypoint}}"), "/usr/bin/java", "entrypoint")
		t.Logf("smoke test passed (OpenJDK %s, javac, jshell, read-only rootfs)", major)
	})
	register("kube-vip", func(t *T) {
		u := t.Docker("inspect", t.Image, "--format", "{{.Config.User}}")
		if v := t.Run("version"); t.Want != "" {
			t.Match(v, `(?m)^Version:  v`+regexp.QuoteMeta(t.Want)+`$`, "version")
		}
		kc := t.DeadKubeconfig()
		home := t.TempDir()
		os.Mkdir(filepath.Join(home, ".kube"), 0o755)
		b, _ := os.ReadFile(kc)
		os.WriteFile(filepath.Join(home, ".kube/config"), b, 0o644)
		name := t.Name("")
		t.Docker("create", "--name", name, "-p", "127.0.0.1:12112:2112", t.Image, "manager", "--services", "--arp",
			"--k8sConfigPath=/kubeconfig", "--interface=eth0", "--prometheusHTTPServer=:2112")
		t.Cleanup(func() { docker("rm", "-f", name) })
		t.Docker("cp", kc, name+":/kubeconfig")
		t.Docker("cp", home+"/.", name+":/home/kube-vip/")
		t.Docker("start", name)
		t.waitFor(30, time.Second, name, "/metrics never answered", func() bool {
			if s := t.Docker("inspect", "-f", "{{.State.Status}}", name); s != "running" {
				t.Fatalf("kube-vip exited\n%s", t.Logs(name))
			}
			_, m := Get("http://127.0.0.1:12112/metrics")
			return regexp.MustCompile(`(?m)^go_goroutines`).MatchString(m)
		})
		t.waitFor(20, time.Second, name, "never tried the API server for its lease", func() bool { return strings.Contains(t.Logs(name), "Error retrieving lease lock") })
		if s := t.Docker("inspect", "-f", "{{.State.Status}}", name); s != "running" {
			t.Fatalf("kube-vip is not running any more\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed (uid %s, retrying a dead API server, /metrics up)", u)
	})
}
