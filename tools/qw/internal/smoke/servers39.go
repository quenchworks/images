package smoke

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

func init() {
	register("freshrss", func(t *T) {
		const b = "http://127.0.0.1:18083"
		const g = b + "/api/greader.php"
		u := t.User("1001")
		vol, name := t.Name("data"), t.Name("")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		data := vol + ":/var/www/FreshRSS/data"
		cli := func(args ...string) string {
			out, err := t.RunFlags([]string{"--read-only", "--tmpfs", "/tmp:uid=1001,gid=1001,mode=1777", "-v", data, "--entrypoint", "php"}, args...)
			if err != nil {
				t.Fatalf("php %v: %s", args, out)
			}
			return out
		}
		cli("./cli/prepare.php")
		cli("./cli/do-install.php", "--default-user", "admin", "--auth-type", "form", "--environment", "production", "--base-url", b,
			"--db-type", "sqlite", "--api-enabled", "--disable-update")
		cli("./cli/create-user.php", "--user", "admin", "--password", "smoke-pass-2026", "--api-password", "smoke-api-2026")
		if out, err := t.RunInputFlags([]string{"-v", vol + ":/d", "--entrypoint", "/bin/sh"}, `<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0"><head><title>smoke</title></head><body>
<outline text="quench-smoke"><outline type="rss" text="Smoke feed" xmlUrl="http://feed.invalid/smoke.xml"/></outline>
</body></opml>
`, "-c", "cat > /d/smoke.opml"); err != nil {
			t.Fatalf("writing the OPML: %s", out)
		}
		cli("./cli/import-for-user.php", "--user", "admin", "--filename", "/var/www/FreshRSS/data/smoke.opml")
		v := cli("-r", `require "constants.php"; echo FRESHRSS_VERSION;`)
		if t.Want != "" && v != t.Want {
			t.Fatalf("freshrss %s, expected %s", v, t.Want)
		}
		start := func() {
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:uid=1001,gid=1001,mode=1777", "-v", data, "-p", "127.0.0.1:18083:8080"})
			t.WaitHTTP(b+"/i/", 60, name)
		}
		login := func(pw string) (int, string) {
			c, r := Post(g+"/accounts/ClientLogin", "application/x-www-form-urlencoded", url.Values{"Email": {"admin"}, "Passwd": {pw}}.Encode())
			m := regexp.MustCompile(`(?m)^Auth=(\S+)`).FindStringSubmatch(r)
			if m == nil {
				return c, ""
			}
			return c, m[1]
		}
		tags := func() string {
			_, tok := login("smoke-api-2026")
			if tok == "" {
				t.Fatalf("no API token")
			}
			_, r := DoH("GET", g+"/reader/api/0/tag/list?output=json", "", map[string]string{"Authorization": "GoogleLogin auth=" + tok})
			return r
		}
		start()
		_, ui := Get(b + "/i/")
		t.ContainsFold(ui, "freshrss", "web UI")
		if c, _ := login("wrong"); c != 401 && c != 403 {
			t.Fatalf("a wrong API password got HTTP %d", c)
		}
		t.Contains(tags(), "quench-smoke", "imported category listed")
		t.Docker("rm", "-f", name)
		start()
		t.Contains(tags(), "quench-smoke", "category kept across a restart")
		if regexp.MustCompile(`(?i)read-only file system|PHP Fatal`).MatchString(t.Logs(name)) {
			t.Fatalf("error in the logs\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed (freshrss %s: CLI install, Google Reader API login, category kept across a restart; user %s)", v, u)
	})
	register("mitmproxy", func(t *T) {
		v := t.RunAny("--version")
		t.Match(v, `(?m)^Mitmproxy: `, "version banner")
		if t.Want != "" {
			t.Match(v, `(?m)^Mitmproxy: `+regexp.QuoteMeta(t.Want)+`$`, "version")
		}
		net_, web, name := t.Name("net"), t.Name("web"), t.Name("")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		ro := []string{"--network", net_, "--read-only", "--tmpfs", "/tmp", "--tmpfs", "/home/mitmproxy:uid=1001"}
		t.Start(web, append(append([]string{}, ro...), "--entrypoint", "/usr/bin/mitmweb"),
			"--mode", "reverse:http://127.0.0.1:8081@8443", "--set", "web_open_browser=false", "--set", "web_password=quench-smoke")
		t.Start(name, append(append([]string{}, ro...), "-p", "127.0.0.1::8080"), "--mode", "regular@8080", "--set", "ssl_insecure=true")
		pu, _ := url.Parse("http://127.0.0.1:" + t.hostPort(name, "8080/tcp"))
		var resp *http.Response
		var body string
		t.waitFor(60, time.Second, name, "no intercepted HTTPS response", func() bool {
			if s := t.Docker("inspect", "-f", "{{.State.Status}}", name); s != "running" {
				t.Fatalf("container died\n%s", t.Logs(name))
			}
			ca, err := docker("exec", name, "/opt/mitmproxy/venv/bin/python", "-c", "print(open('/home/mitmproxy/.mitmproxy/mitmproxy-ca-cert.pem').read(), end='')")
			pool := x509.NewCertPool()
			if err != nil || !pool.AppendCertsFromPEM([]byte(ca)) {
				return false
			}
			c := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(pu), TLSClientConfig: &tls.Config{RootCAs: pool}}}
			r, err := c.Get("https://" + web + ":8443/")
			if err != nil {
				return false
			}
			b, _ := io.ReadAll(r.Body)
			r.Body.Close()
			resp, body = r, string(b)
			return strings.Contains(strings.ToLower(body), "mitmproxy")
		})
		if iss := resp.TLS.PeerCertificates[0].Issuer.String(); !strings.Contains(strings.ToLower(iss), "mitmproxy") {
			t.Fatalf("the certificate was not issued by mitmproxy: %s", iss)
		}
		for _, c := range []string{name, web} {
			if regexp.MustCompile(`(?i)read-only file system|permission denied|Traceback`).MatchString(t.Logs(c)) {
				t.Fatalf("errors in the logs of %s\n%s", c, t.Logs(c))
			}
		}
		t.Logf("smoke test passed (nonroot %s, TLS interception verified against mitmproxy's CA, mitmweb through the proxy)", t.User("1001"))
	})
	register("mlflow", func(t *T) {
		t.User("1001")
		v := t.RunEntry("/usr/bin/mlflow", "--version")
		t.Match(v, `^mlflow, version `, "version line")
		if t.Want != "" && v != "mlflow, version "+t.Want {
			t.Fatalf("%s, expected %s", v, t.Want)
		}
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp", "-p", "127.0.0.1:15000:5000"},
			"server", "--backend-store-uri", "sqlite:////tmp/mlflow.db", "--default-artifact-root", "/tmp/artifacts", "--host", "0.0.0.0", "--port", "5000")
		t.waitFor(60, time.Second, name, "/health did not return 200", func() bool {
			if s := t.Docker("inspect", "-f", "{{.State.Status}}", name); s != "running" {
				t.Fatalf("server container exited during boot\n%s", t.Logs(name))
			}
			c, _ := Get("http://127.0.0.1:15000/health")
			return c == 200
		})
		if c, _ := Get("http://127.0.0.1:15000/api/2.0/mlflow/experiments/search?max_results=10"); c != 200 {
			t.Fatalf("experiments API returned %d\n%s", c, t.Logs(name))
		}
		t.Logf("smoke test passed (/health, experiments API, uid 1001)")
	})
	register("spire", func(t *T) {
		const td = "spiffe://example.org"
		var v string
		for _, b := range []string{"spire-server", "spire-agent"} {
			v = lastLine(t.RunEntryAny("/usr/bin/"+b, "--version"))
			if v == "" || strings.Contains(v, "dev") || (t.Want != "" && v != t.Want) {
				t.Fatalf("%s version %q, expected %s", b, v, t.Want)
			}
		}
		net_, srv, agent := t.Name("net"), t.Name("server"), t.Name("agent")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Start(srv, []string{"--network", net_, "--network-alias", "spire-server", "-p", "127.0.0.1:18080:8080"})
		t.WaitHTTP("http://127.0.0.1:18080/ready", 60, srv)
		s := func(args ...string) string {
			return t.Docker(append([]string{"exec", srv, "/usr/bin/spire-server"}, args...)...)
		}
		m := regexp.MustCompile(`(?m)^Token: (\S+)`).FindStringSubmatch(s("token", "generate", "-spiffeID", td+"/node1"))
		if m == nil {
			t.Fatalf("no join token")
		}
		s("entry", "create", "-parentID", td+"/node1", "-spiffeID", td+"/workload", "-selector", "unix:uid:1001")
		t.Start(agent, []string{"--network", net_, "-p", "127.0.0.1:18081:8080", "--entrypoint", "/usr/bin/spire-agent"}, "run", "-config", "/etc/spire/agent/agent.conf", "-joinToken", m[1])
		t.WaitHTTP("http://127.0.0.1:18081/ready", 60, agent)
		t.waitFor(30, 2*time.Second, agent, "workload never got an SVID", func() bool {
			out, err := docker("exec", agent, "/usr/bin/spire-agent", "api", "fetch", "x509", "-socketPath", "/tmp/spire-agent/public/api.sock")
			return err == nil && strings.Contains(out, td+"/workload")
		})
		t.Docker("restart", srv)
		t.WaitHTTP("http://127.0.0.1:18080/ready", 60, srv)
		t.Contains(s("entry", "show", "-spiffeID", td+"/workload"), td+"/workload", "entry kept across a server restart")
		t.Logf("smoke test passed (version %s, join token, agent, X.509 SVID, entry kept across restart; user %s)", v, t.Docker("inspect", t.Image, "--format", "{{.Config.User}}"))
	})
}
