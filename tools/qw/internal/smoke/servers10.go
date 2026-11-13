package smoke

import (
	"fmt"
	"io"
	"net"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// dnsQuery sends one A query with the DNSSEC OK bit over UDP and returns the reply.
func dnsQuery(server, host string) (m *dnsmessage.Message, err error) {
	for range 3 { // a validating resolver can take over 10s to SERVFAIL a bad signature
		if m, err = dnsQuery1(server, host); err == nil {
			return m, nil
		}
	}
	return m, err
}

func dnsQuery1(server, host string) (*dnsmessage.Message, error) {
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: 0x5157, RecursionDesired: true})
	b.EnableCompression()
	b.StartQuestions()
	b.Question(dnsmessage.Question{Name: dnsmessage.MustNewName(host + "."), Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET})
	b.StartAdditionals()
	var opt dnsmessage.ResourceHeader
	opt.SetEDNS0(4096, dnsmessage.RCodeSuccess, true)
	b.OPTResource(opt, dnsmessage.OPTResource{})
	q, err := b.Finish()
	if err != nil {
		return nil, err
	}
	c, err := net.DialTimeout("udp", server, 5*time.Second)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err := c.Write(q); err != nil {
		return nil, err
	}
	buf := make([]byte, 4096)
	n, err := c.Read(buf)
	if err != nil {
		return nil, err
	}
	var m dnsmessage.Message
	return &m, m.Unpack(buf[:n])
}

// harborBinary checks a Harbor component: release string in the static binary, then a
// startup that fails on missing config the way the component does.
func harborBinary(bin, startup string) func(*T) {
	return func(t *T) {
		if t.Want == "" {
			t.Fatalf("usage: qw smoke %s <image-ref> <app-version>", t.App)
		}
		if out, err := t.RunFlags([]string{"--entrypoint", "/bin/sh"}, "-c", "strings "+bin+" | grep -q 'v"+t.Want+"'"); err != nil {
			t.Fatalf("release string v%s not found in %s %s", t.Want, bin, out)
		}
		if out, err := t.RunFlags([]string{"--entrypoint", "/bin/sh"}, "-c", "test -x "+bin+" && ! ldd "+bin+" 2>/dev/null | grep -q '=>'"); err != nil {
			t.Fatalf("%s is not a static executable: %s", bin, out)
		}
		out := t.RunAny()
		t.Match(out, startup, "startup output")
		if regexp.MustCompile(`(?i)no such file or directory: ` + bin + `|exec format error`).MatchString(out) {
			t.Fatalf("binary failed to exec (wrong arch / not static)")
		}
		t.Logf("smoke test passed (v%s, static, nonroot uid %s)", t.Want, t.User("1001"))
	}
}

func init() {
	register("harbor-core", harborBinary("/usr/bin/harbor_core", `(?i)app.conf|config|annotation parser|registered`))
	register("harbor-exporter", harborBinary("/usr/bin/harbor_exporter", `(?i)database|annotation parser|registered|exporter`))
	register("harbor-jobservice", harborBinary("/usr/bin/harbor_jobservice", `(?i)config|annotation parser|registered|jobservice`))
	register("harbor-registryctl", harborBinary("/usr/bin/harbor_registryctl", `(?i)Config file should be specified|Specify registryCtl|registryCtl configuration`))
	register("unbound", func(t *T) {
		u := t.User("1001")
		v := strings.SplitN(t.RunAny("-V"), "\n", 2)[0]
		if t.Want != "" {
			t.Contains(v, "Version "+t.Want, "version")
		}
		t.RunEntry("/usr/bin/unbound-checkconf", "/etc/unbound/unbound.conf")
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp", "-p", "127.0.0.1:15053:5053/udp", "-p", "127.0.0.1:15053:5053/tcp"})
		const srv = "127.0.0.1:15053"
		for i := 0; ; i++ {
			if m, err := dnsQuery(srv, "example.com"); err == nil && len(m.Answers) > 0 {
				break
			}
			if i == 30 {
				t.Fatalf("no answer for example.com\n%s", t.Logs(name))
			}
			time.Sleep(time.Second)
		}
		m, err := dnsQuery(srv, "isc.org")
		if err != nil || !m.AuthenticData {
			t.Fatalf("isc.org not DNSSEC-authenticated (%v)", err)
		}
		if m, err := dnsQuery(srv, "dnssec-failed.org"); err != nil || m.RCode != dnsmessage.RCodeServerFailure {
			t.Fatalf("dnssec-failed.org was not rejected (%v %v)", err, m)
		}
		t.Logf("smoke test passed (isc.org validated, bad signature SERVFAIL; uid %s)", u)
	})
	register("wordpress", func(t *T) {
		u := t.User("1001")
		t.Contains(t.RunEntry("/usr/bin/php", "-r", `require "/var/www/html/wp-includes/version.php"; echo "wp_version=".$wp_version."\n";`), "wp_version=", "version.php")
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp", "-p", "127.0.0.1:8099:8080"})
		for i := 0; ; i++ {
			if c, _ := Get("http://127.0.0.1:8099/"); c == 200 {
				break // the client follows the 302 to setup-config.php
			}
			if i == 30 {
				t.Fatalf("WordPress did not answer over HTTP\n%s", t.Logs(name))
			}
			time.Sleep(time.Second)
		}
		c, body := Get("http://127.0.0.1:8099/wp-admin/setup-config.php")
		if c != 200 {
			t.Fatalf("setup-config.php HTTP %d\n%s", c, t.Logs(name))
		}
		t.ContainsFold(body, "wordpress", "setup page")
		t.Logf("smoke test passed (php-fpm+nginx up, setup wizard served, nonroot user: %s)", u)
	})
	register("zookeeper", func(t *T) {
		name := t.Name("")
		t.Start(name, []string{"-p", "127.0.0.1:2181:2181"})
		fourlw := func(cmd string) string {
			c, err := net.DialTimeout("tcp", "127.0.0.1:2181", 3*time.Second)
			if err != nil {
				return ""
			}
			defer c.Close()
			c.SetDeadline(time.Now().Add(3 * time.Second))
			fmt.Fprint(c, cmd)
			b, _ := io.ReadAll(c)
			return string(b)
		}
		for i := 0; fourlw("ruok") != "imok"; i++ {
			if i == 40 {
				t.Fatalf("zookeeper did not become ready\n%s", t.Logs(name))
			}
			time.Sleep(time.Second)
		}
		t.Match(fourlw("srvr"), `(?i)Mode:|Zookeeper version`, "srvr")
		t.Logf("smoke test passed (nonroot user: %s)", t.User("1001"))
	})
	register("kvrocks", func(t *T) {
		const pw, cliImg = "quench-smoke-pw", "ghcr.io/quenchworks/images/valkey:9.1.2"
		u := t.User("1001")
		net_, name, vol := t.Name("net"), t.Name(""), t.Name("data")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Cleanup(func() { docker("volume", "rm", vol) })
		start := func() {
			t.Start(name, []string{"--network", net_, "--read-only", "--tmpfs", "/tmp", "-v", vol + ":/var/lib/kvrocks"}, "--requirepass", pw)
		}
		cli := func(args ...string) string {
			out, _ := docker(append([]string{"run", "--rm", "--network", net_, "--entrypoint", "/usr/bin/valkey-cli", cliImg, "-h", name, "-p", "6666", "-a", pw, "--no-auth-warning"}, args...)...)
			return strings.ReplaceAll(out, "\r", "")
		}
		up := func(what string) {
			for i := 0; cli("PING") != "PONG"; i++ {
				if i == 60 {
					t.Fatalf("kvrocks %s\n%s", what, t.Logs(name))
				}
				time.Sleep(time.Second)
			}
		}
		start()
		up("never answered PING")
		m := regexp.MustCompile(`(?m)^kvrocks_version:(\S+)`).FindStringSubmatch(cli("INFO", "server"))
		if m == nil || (t.Want != "" && m[1] != t.Want) {
			t.Fatalf("kvrocks_version %v, expected %s", m, t.Want)
		}
		if cli("SET", "k", "quench") != "OK" || cli("GET", "k") != "quench" {
			t.Fatalf("SET/GET failed")
		}
		cli("HSET", "h", "f1", "v1", "f2", "v2")
		if cli("HGET", "h", "f2") != "v2" {
			t.Fatalf("hash round trip failed")
		}
		cli("RPUSH", "l", "a", "b", "c")
		if cli("LRANGE", "l", "0", "-1") != "a\nb\nc" {
			t.Fatalf("list round trip failed")
		}
		if out, _ := docker("run", "--rm", "--network", net_, "--entrypoint", "/usr/bin/valkey-cli", cliImg, "-h", name, "-p", "6666", "PING"); strings.HasPrefix(out, "PONG") {
			t.Fatalf("server answered without the password")
		}
		t.Docker("rm", "-f", name)
		start()
		up("did not come back")
		if cli("GET", "k") != "quench" {
			t.Fatalf("data lost across a restart")
		}
		t.Logf("smoke test passed (kvrocks %s, uid %s, auth, strings/hashes/lists, data kept across restart)", m[1], u)
	})
	register("linkerd-control-plane", func(t *T) {
		name := t.Name("")
		t.Start(name, []string{"-p", "127.0.0.1:9996:9996"}, "-enable-endpoint-slices=false", "-enable-ipv6=false")
		time.Sleep(5 * time.Second)
		logs := t.Logs(name)
		t.Contains(logs, "starting admin server on :9996", "reached the admin server")
		t.Contains(logs, "Failed to initialize K8s API", "failed on the unreachable API server")
		if m := regexp.MustCompile(`panic:|SIGSEGV|no such file or directory|cannot execute`).FindString(logs); m != "" {
			t.Fatalf("the binary did not start cleanly (%s):\n%s", m, logs)
		}
		if t.Want != "" {
			t.Contains(logs, "running version "+t.Want, "version")
		}
		t.Logf("boot test passed (nonroot user: %s)", t.User("1001"))
	})
	register("mosquitto", func(t *T) {
		u := t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"-p", "18831:1883"})
		mq := func(run []string, bin string, args ...string) (string, error) {
			return docker(append(append(run, "--network", "host", "--entrypoint", bin, t.Image, "-h", "127.0.0.1", "-p", "18831"), args...)...)
		}
		for i := 0; i < 20; i++ {
			if _, err := mq([]string{"run", "--rm"}, "mosquitto_sub", "-t", "$SYS/#", "-C", "1", "-W", "2"); err == nil {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		sub := t.Name("sub")
		if out, err := mq([]string{"run", "-d", "--name", sub}, "mosquitto_sub", "-t", "quench/test", "-C", "1", "-W", "5"); err != nil {
			t.Fatalf("subscriber: %s", out)
		}
		t.Cleanup(func() { docker("rm", "-f", sub) })
		time.Sleep(time.Second)
		if out, err := mq([]string{"run", "--rm"}, "mosquitto_pub", "-t", "quench/test", "-m", "hello-quench"); err != nil {
			t.Fatalf("publish: %s", out)
		}
		t.Docker("wait", sub)
		if got := t.Logs(sub); got != "hello-quench" {
			t.Fatalf("pub/sub round-trip failed (got '%s')\n%s", got, t.Logs(name))
		}
		t.Logf("smoke test passed (nonroot user: %s, MQTT pub/sub OK)", u)
	})
}
