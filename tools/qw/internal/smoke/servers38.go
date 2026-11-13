package smoke

import (
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// aRecords returns the A answers of m as dotted strings.
func aRecords(m *dnsmessage.Message) []string {
	var out []string
	for _, a := range m.Answers {
		if r, ok := a.Body.(*dnsmessage.AResource); ok {
			out = append(out, net.IP(r.A[:]).String())
		}
	}
	return out
}

func init() {
	register("powerdns", func(t *T) {
		const srv = "127.0.0.1:15353"
		v := regexp.MustCompile(`(?i)PowerDNS Authoritative Server.*`).FindString(t.RunEntryAny("/usr/bin/pdns_server", "--version"))
		t.Match(v, `[0-9]+\.[0-9]+\.[0-9]+`, "version")
		if t.Want != "" {
			t.Contains(v, t.Want, "version")
		}
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/var/lib/powerdns:rw,uid=1001,gid=1001", "--tmpfs", "/tmp:rw,mode=1777",
			"-p", "127.0.0.1:15353:5353/udp", "-p", "127.0.0.1:15353:5353/tcp"})
		t.waitFor(30, time.Second, name, "pdns_server never finished starting", func() bool {
			if s := t.Docker("inspect", "-f", "{{.State.Status}}", name); s != "running" {
				t.Fatalf("container died during startup\n%s", t.Logs(name))
			}
			return strings.Contains(t.Logs(name), "Done launching threads")
		})
		t.Docker("exec", name, "/usr/bin/pdnsutil", "--config-dir=/etc/powerdns", "create-zone", "quench.test", "ns1.quench.test")
		t.Docker("exec", name, "/usr/bin/pdnsutil", "--config-dir=/etc/powerdns", "add-record", "quench.test", "www.quench.test.", "A", "3600", "192.0.2.10")
		t.Docker("exec", name, "/usr/bin/pdns_control", "--config-dir=/etc/powerdns", "rediscover")
		for _, network := range []string{"udp", "tcp"} {
			var m *dnsmessage.Message
			t.waitFor(10, time.Second, name, "no A record over "+network, func() bool {
				var err error
				m, err = dnsAsk(network, srv, "www.quench.test", dnsmessage.TypeA)
				return err == nil && strings.Join(aRecords(m), ",") == "192.0.2.10"
			})
			if !m.Authoritative {
				t.Fatalf("answer over %s is not authoritative", network)
			}
		}
		m, err := dnsAsk("udp", srv, "example.com", dnsmessage.TypeA)
		if err != nil || m.RCode != dnsmessage.RCodeRefused {
			t.Fatalf("a name outside every zone was not REFUSED (%v)", err)
		}
		if m := regexp.MustCompile(`Read-only file system|[Pp]ermission denied|Fatal|Exiting`).FindString(t.Logs(name)); m != "" {
			t.Fatalf("errors in logs (%s)\n%s", m, t.Logs(name))
		}
		t.Logf("smoke test passed (nonroot %s, authoritative answers over UDP and TCP, out-of-zone REFUSED)", t.User("1001"))
	})
	register("dnscrypt-proxy", func(t *T) {
		const srv = "127.0.0.1:15353"
		u := t.User("1001")
		if v := t.RunAny("-version"); t.Want != "" && v != t.Want {
			t.Fatalf("dnscrypt-proxy %s, expected %s", v, t.Want)
		}
		w := t.TempDir()
		os.WriteFile(filepath.Join(w, "dnscrypt-proxy.toml"), []byte(`listen_addresses = ['0.0.0.0:5353']
server_names = ['cloudflare']
cloaking_rules = '/c/cloaking.txt'
[blocked_names]
blocked_names_file = '/c/blocked.txt'
[static.cloudflare]
stamp = 'sdns://AgcAAAAAAAAABzEuMC4wLjEAEmRucy5jbG91ZGZsYXJlLmNvbQovZG5zLXF1ZXJ5'
`), 0o644)
		os.WriteFile(filepath.Join(w, "cloaking.txt"), []byte("smoke.quench.test 192.0.2.10\n"), 0o644)
		os.WriteFile(filepath.Join(w, "blocked.txt"), []byte("blocked.quench.test\n"), 0o644)
		name := t.Name("")
		t.Start(name, []string{"--read-only", "-v", w + ":/c:ro", "-p", "127.0.0.1:15353:5353/udp"}, "-config", "/c/dnscrypt-proxy.toml")
		t.waitFor(30, time.Second, name, "cloaked name not answered", func() bool {
			m, err := dnsAsk("udp", srv, "smoke.quench.test", dnsmessage.TypeA)
			return err == nil && strings.Join(aRecords(m), ",") == "192.0.2.10"
		})
		if m, err := dnsAsk("udp", srv, "blocked.quench.test", dnsmessage.TypeA); err == nil && m.RCode == dnsmessage.RCodeSuccess {
			if a := aRecords(m); len(a) > 0 && a[0] != "0.0.0.0" {
				t.Fatalf("blocked name answered: %v", a)
			}
		}
		t.waitFor(20, time.Second, name, "upstream resolution failed", func() bool {
			m, err := dnsAsk("udp", srv, "one.one.one.one", dnsmessage.TypeA)
			if err != nil || m.RCode != dnsmessage.RCodeSuccess {
				return false
			}
			a := aRecords(m)
			return len(a) > 0 && (a[0] == "1.1.1.1" || a[0] == "1.0.0.1")
		})
		if regexp.MustCompile(`(?i)panic|read-only file system`).MatchString(t.Logs(name)) {
			t.Fatalf("crashed or wrote to the root\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed: cloaking, blocking, DoH upstream, user %s", u)
	})
	register("drupal", func(t *T) {
		const b = "http://127.0.0.1:8098"
		u := t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777,uid=1001,gid=1001", "--tmpfs", "/opt/drupal/sites/default/files:rw,uid=1001,gid=1001", "-p", "127.0.0.1:8098:8080"})
		t.waitCode(b+"/", 60, time.Second, name, 200)
		c, page := Get(b + "/core/install.php")
		if c != 200 {
			t.Fatalf("install.php: HTTP %d\n%s", c, t.Logs(name))
		}
		t.ContainsFold(page, "drupal", "install page")
		mods := t.Docker("exec", name, "php", "-m")
		for _, e := range strings.Fields("gd pdo pdo_mysql pdo_pgsql pdo_sqlite mbstring dom simplexml xml ctype curl openssl fileinfo") {
			t.Match(mods, `(?mi)^`+e+`$`, "PHP extension "+e)
		}
		t.ContainsFold(mods, "opcache", "OPcache")
		t.Docker("exec", name, "sh", "-c", `grep -m1 "const VERSION" /opt/drupal/core/lib/Drupal.php`)
		for _, bin := range []string{"php-fpm", "nginx", "supervisord"} {
			if _, err := docker("exec", name, "sh", "-c", "command -v "+bin); err != nil {
				t.Fatalf("runtime tool %s missing", bin)
			}
		}
		t.Logf("smoke test passed (nonroot uid %s, nginx + php-fpm serve the Drupal installer)", u)
	})
	register("fluent-bit", func(t *T) {
		const b = "http://127.0.0.1:2020"
		name, _ := t.httpUp([]string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-p", "127.0.0.1:2020:2020"}, b+"/api/v1/health", 30)
		if c, _ := Get(b + "/"); c != 200 {
			t.Fatalf("/ returned %d", c)
		}
		t.WaitHTTP(b+"/api/v1/metrics", 10, name)
		if regexp.MustCompile(`(?i)read-only file system|permission denied`).MatchString(t.Logs(name)) {
			t.Fatalf("read-only/permission errors in logs\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed (nonroot user: %s)", t.User("1001"))
	})
}
