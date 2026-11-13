package smoke

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/proxy"
)

// waitCount polls a container's logs until re matches at least n times (restarts append).
func (t *T) waitCount(name, re string, n, tries int) {
	r := regexp.MustCompile(re)
	for i := 0; i < tries; i++ {
		if len(r.FindAllString(t.Logs(name), -1)) >= n {
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("broker never started\n%s", t.Logs(name))
}

// hostPort returns the host port docker published for a container port.
func (t *T) hostPort(name, port string) string {
	first := strings.SplitN(t.Docker("port", name, port), "\n", 2)[0]
	return first[strings.LastIndex(first, ":")+1:]
}

func viaProxy(proxyURL, target string) (int, string) {
	u, _ := url.Parse(proxyURL)
	c := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(u)}}
	r, err := c.Get(target)
	if err != nil {
		return 0, err.Error()
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return r.StatusCode, string(b)
}

func init() {
	register("activemq-artemis", func(t *T) {
		etc := t.TempDir()
		os.WriteFile(filepath.Join(etc, "artemis-users.properties"), []byte("smoke = smokepw\n"), 0o644)
		os.WriteFile(filepath.Join(etc, "artemis-roles.properties"), []byte("amq = smoke\n"), 0o644)
		name := t.Name("")
		t.Start(name, []string{"-v", etc + "/artemis-users.properties:/opt/artemis-instance/etc/artemis-users.properties:ro",
			"-v", etc + "/artemis-roles.properties:/opt/artemis-instance/etc/artemis-roles.properties:ro"})
		const started = "AMQ221007: Server is now active"
		t.waitCount(name, started, 1, 90)
		lines := regexp.MustCompile(`Apache Artemis Message Broker version [0-9.]+`).FindAllString(t.Logs(name), -1)
		line := lines[len(lines)-1]
		t.Logf("%s", line)
		if t.Want != "" && !strings.HasSuffix(line, "version "+t.Want) {
			t.Fatalf("expected %s", t.Want)
		}
		cli := func(args ...string) string {
			a := append([]string{"exec", name, "/usr/lib/jvm/java-21-openjdk/bin/java", "-Dartemis.home=/opt/artemis",
				"-Dartemis.instance=/opt/artemis-instance", "-Djava.io.tmpdir=/tmp", "-classpath", "/opt/artemis/lib/artemis-boot.jar",
				"org.apache.activemq.artemis.boot.Artemis"}, args...)
			out, _ := docker(append(a, "--url", "tcp://localhost:61616", "--destination", "queue://smoke")...)
			return out
		}
		t.Contains(cli("producer", "--user", "smoke", "--password", "smokepw", "--message-count", "5"), "Produced: 5 messages", "producer")
		t.Docker("restart", name)
		t.waitCount(name, started, 2, 90)
		t.Contains(cli("consumer", "--user", "smoke", "--password", "smokepw", "--message-count", "5", "--receive-timeout", "20000"), "Consumed: 5 messages", "consumer after restart")
		bad := cli("producer", "--user", "smoke", "--password", "wrong", "--message-count", "1")
		if strings.Contains(bad, "Produced: 1 messages") {
			t.Fatalf("a wrong password was accepted")
		}
		t.Match(bad, `(?i)AMQ229031|security|authentication|credentials`, "wrong password refused")
		t.Logf("smoke test passed (%s, nonroot user: %s, persistent messages across a restart)", line, t.User("1001"))
	})
	register("activemq", func(t *T) {
		name := t.Name("")
		t.Start(name, nil)
		const started = `Apache ActiveMQ [0-9.]+ .* started`
		t.waitCount(name, started, 1, 90)
		lines := regexp.MustCompile(`Apache ActiveMQ [0-9.]+ \([^)]*\) started`).FindAllString(t.Logs(name), -1)
		line := lines[len(lines)-1]
		t.Logf("%s", line)
		if t.Want != "" {
			t.Contains(line, "ActiveMQ "+t.Want+" ", "version")
		}
		cli := func(args ...string) string {
			a := append([]string{"exec", name, "/usr/lib/jvm/java-21-openjdk/bin/java", "-Dactivemq.home=/opt/activemq",
				"-Dactivemq.base=/opt/activemq", "-Dactivemq.conf=/opt/activemq/conf", "-Dactivemq.data=/tmp/cli",
				"-jar", "/opt/activemq/bin/activemq.jar"}, args...)
			out, _ := docker(append(a, "--brokerUrl", "tcp://localhost:61616", "--destination", "queue://smoke", "--messageCount", "5")...)
			return out
		}
		t.Match(cli("producer", "--persistent", "true"), `(?i)produced:? *5|5 messages`, "producer")
		t.Docker("restart", name)
		t.waitCount(name, started, 2, 90)
		t.Match(cli("consumer", "--receiveTimeout", "20000"), `(?i)consumed:? *5|5 messages`, "consumer after restart (KahaDB)")
		t.Logf("smoke test passed (%s, nonroot user: %s, persistent messages across a restart)", line, t.User("1001"))
	})
	register("actual", func(t *T) {
		u := t.User("1001")
		ver := t.RunEntry("/usr/bin/node", "-p", `require("/app/node_modules/@actual-app/sync-server/package.json").version`)
		t.Logf("actual %s", ver)
		if t.Want != "" && ver != t.Want {
			t.Fatalf("expected %s", t.Want)
		}
		name, base := t.Name(""), "http://127.0.0.1:15006"
		t.Docker("volume", "create", name)
		t.Cleanup(func() { docker("volume", "rm", name) })
		start := func() {
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:uid=1001,gid=1001", "-v", name + ":/data", "-p", "127.0.0.1:15006:5006"})
			t.WaitHTTP(base+"/health", 60, name)
		}
		j := map[string]string{"Content-Type": "application/json"}
		start()
		_, page := Get(base + "/")
		t.ContainsFold(page, "<html", "web client served")
		if c, b := DoH("POST", base+"/account/bootstrap", `{"password":"smoke-pass-2026"}`, j); c != 200 {
			t.Fatalf("bootstrap: %d %s", c, b)
		}
		_, bad := DoH("POST", base+"/account/login", `{"loginMethod":"password","password":"wrong"}`, j)
		t.Contains(bad, `"status":"error"`, "wrong password refused")
		login := func() string {
			_, b := DoH("POST", base+"/account/login", `{"loginMethod":"password","password":"smoke-pass-2026"}`, j)
			m := regexp.MustCompile(`"token":"([^"]+)"`).FindStringSubmatch(b)
			if m == nil {
				t.Fatalf("login: %s", b)
			}
			return m[1]
		}
		if c, b := DoH("POST", base+"/sync/upload-user-file", "quench-smoke-budget", map[string]string{"x-actual-token": login(),
			"Content-Type": "application/encrypted-file", "x-actual-file-id": "quench-smoke-file", "x-actual-name": "quench-smoke", "x-actual-format": "2"}); c != 200 {
			t.Fatalf("upload: %d %s", c, b)
		}
		files := func() string {
			_, b := DoH("GET", base+"/sync/list-user-files", "", map[string]string{"x-actual-token": login()})
			return b
		}
		t.Contains(files(), "quench-smoke", "file listed")
		t.Docker("rm", "-f", name)
		start()
		t.Contains(files(), "quench-smoke", "file kept across a restart")
		if regexp.MustCompile(`(?i)read-only file system|EROFS`).MatchString(t.Logs(name)) {
			t.Fatalf("wrote to the root\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed (actual %s: bootstrap, login, budget file kept across a restart; user %s)", ver, u)
	})
	register("bifrost", func(t *T) {
		if t.Want == "" {
			t.Fatalf("usage: qw smoke bifrost <image-ref> <version>")
		}
		u := t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"-p", "18080:8080"})
		body := t.WaitHTTP("http://127.0.0.1:18080/", 60, name)
		t.Match(body, `(?i)<div id="root"|bifrost`, "embedded console markup")
		t.Contains(t.Logs(name), t.Want, "version in the startup banner")
		t.Logf("smoke test passed (nonroot user: %s, console served, version %s)", u, t.Want)
	})
	register("airflow", func(t *T) {
		u := t.User("1001")
		ro := []string{"--read-only", "--tmpfs", "/tmp:mode=1777", "--tmpfs", "/opt/airflow:mode=1777"}
		out, err := t.RunFlags(ro, "version")
		if err != nil {
			t.Fatalf("version: %s", out)
		}
		t.Match(out, `(?m)^[0-9]+\.[0-9]+\.[0-9]+$`, "version line")
		if t.Want != "" && out != t.Want {
			t.Fatalf("expected airflow %s, got %s", t.Want, out)
		}
		imp, err := t.RunFlags(append(ro, "--entrypoint", "/opt/airflow-venv/bin/python"), "-c", "import airflow; print(airflow.__version__)")
		if err != nil {
			t.Fatalf("import airflow: %s", imp)
		}
		if info, err := t.RunFlags(ro, "info"); err != nil {
			t.Fatalf("airflow info exited nonzero:\n%s", info)
		}
		t.Logf("smoke test passed (airflow %s: version, import, info; nonroot %s; read-only root)", out, u)
	})
	register("3proxy", func(t *T) {
		u := t.User("1001")
		net_, px, be, def := t.Name("net"), t.Name(""), t.Name("be"), t.Name("def")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Cleanup(func() { docker("rm", "-f", be, def) })
		t.Docker("run", "-d", "--name", be, "--network", net_, "--entrypoint", "/usr/bin/caddy",
			"ghcr.io/quenchworks/images/caddy:2.11.9", "respond", "--listen", ":8000", "quench-backend")
		cfg := filepath.Join(t.TempDir(), "3proxy.cfg")
		os.WriteFile(cfg, []byte("log\nusers quench:CL:quench-pass\nauth strong\nallow quench\nproxy -p3128\nsocks -p1080\n"), 0o644)
		t.Docker("create", "--name", px, "--network", net_, "-p", "127.0.0.1::3128", "-p", "127.0.0.1::1080", t.Image, "/tmp/3proxy.cfg")
		t.Cleanup(func() { docker("rm", "-f", px) })
		t.Docker("cp", cfg, px+":/tmp/3proxy.cfg")
		t.Docker("start", px)
		hp, sp := t.hostPort(px, "3128/tcp"), t.hostPort(px, "1080/tcp")
		target := "http://" + be + ":8000/"
		var out string
		for i := 0; ; i++ {
			if _, out = viaProxy("http://quench:quench-pass@127.0.0.1:"+hp, target); out == "quench-backend" {
				break
			}
			if i == 20 {
				t.Fatalf("HTTP proxy fetch failed: %s\n%s", out, t.Logs(px))
			}
			time.Sleep(time.Second)
		}
		if c, _ := viaProxy("http://127.0.0.1:"+hp, target); c != 407 {
			t.Fatalf("no credentials got HTTP %d, want 407", c)
		}
		d, _ := proxy.SOCKS5("tcp", "127.0.0.1:"+sp, &proxy.Auth{User: "quench", Password: "quench-pass"}, proxy.Direct)
		sc := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{DialContext: func(_ context.Context, n, a string) (net.Conn, error) { return d.Dial(n, a) }}}
		r, err := sc.Get(target)
		if err != nil {
			t.Fatalf("SOCKS5 fetch failed: %v", err)
		}
		b, _ := io.ReadAll(r.Body)
		r.Body.Close()
		if string(b) != "quench-backend" {
			t.Fatalf("SOCKS5 fetch failed: %s", b)
		}
		t.Docker("run", "-d", "--name", def, "--network", net_, "-p", "127.0.0.1::3128", t.Image)
		dp := t.hostPort(def, "3128/tcp")
		time.Sleep(2 * time.Second)
		if c, _ := viaProxy("http://127.0.0.1:"+dp, target); c != 407 {
			t.Fatalf("default config let a request through (HTTP %d)\n%s", c, t.Logs(def))
		}
		t.Logf("smoke test passed (http + socks5 auth, default config refuses; nonroot user: %s)", u)
	})
}
