package smoke

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

func init() {
	// The recursor answers a local zone (recursor.auth_zones) so the test needs no internet: A over
	// UDP and TCP, NXDOMAIN from the zone's SOA, and rec_control ping over the socket in /tmp, on a
	// read-only rootfs.
	register("powerdns-recursor", func(t *T) {
		u := t.User("1001")
		v := t.RunEntryAny("/usr/bin/pdns_recursor", "--version")
		t.Match(v, `Recursor [0-9]+\.[0-9]+\.[0-9]+`, "recursor version")
		if t.Want != "" {
			t.Contains(v, "Recursor "+t.Want, "recursor version")
		}
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "recursor.yml"), []byte(`incoming:
  listen: ['0.0.0.0:5353']
  allow_from: ['0.0.0.0/0']
recursor:
  socket_dir: /tmp
  daemon: false
  auth_zones:
    - zone: quench.test
      file: /etc/powerdns/quench.test.zone
logging:
  disable_syslog: true
`), 0o644)
		os.WriteFile(filepath.Join(dir, "quench.test.zone"), []byte(`$ORIGIN quench.test.
$TTL 300
@   IN SOA ns1 hostmaster 1 3600 600 86400 300
@   IN NS  ns1
ns1 IN A   192.0.2.1
www IN A   192.0.2.10
`), 0o644)
		name := t.Name("")
		// host side 15354: 5353 is mDNS on most desktops
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777",
			"-v", dir + "/recursor.yml:/etc/powerdns/recursor.yml:ro", "-v", dir + "/quench.test.zone:/etc/powerdns/quench.test.zone:ro",
			"-p", "127.0.0.1:15354:5353/udp", "-p", "127.0.0.1:15354:5353/tcp"})
		const srv = "127.0.0.1:15354"
		for _, proto := range []string{"udp", "tcp"} {
			t.waitFor(30, time.Second, name, "no A record for www.quench.test over "+proto, func() bool {
				if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
					t.Fatalf("recursor died during startup\n%s", t.Logs(name))
				}
				m, err := dnsAsk(proto, srv, "www.quench.test", dnsmessage.TypeA)
				return err == nil && slices.Contains(aRecords(m), "192.0.2.10")
			})
		}
		if m, err := dnsAsk("udp", srv, "missing.quench.test", dnsmessage.TypeA); err != nil || m.RCode != dnsmessage.RCodeNameError {
			t.Fatalf("missing.quench.test was not NXDOMAIN (%v, %+v)", err, m)
		}
		// 5.x answers once per thread ("pong worker", "pong tcpworker", "pong task")
		pong, _ := docker("exec", name, "/usr/bin/rec_control", "--socket-dir=/tmp", "ping")
		for _, l := range strings.Split(strings.TrimSpace(pong), "\n") {
			if !strings.HasPrefix(l, "pong") {
				t.Fatalf("rec_control did not answer pong from every thread: %q", pong)
			}
		}
		if regexp.MustCompile(`Read-only file system|[Pp]ermission denied|Fatal|Exiting`).MatchString(t.Logs(name)) {
			t.Fatalf("errors in the logs\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed (powerdns-recursor %s: read-only rootfs, A over UDP and TCP, NXDOMAIN, rec_control pong; user %s)", t.Want, u)
	})

	// CouchDB on a read-only rootfs with tmpfs state: welcome, system databases, _up, a document
	// roundtrip and a JS map view (proves the bundled QuickJS view server links and runs).
	register("couchdb", func(t *T) {
		const base = "http://127.0.0.1:15984"
		auth := map[string]string{"Authorization": "Basic YWRtaW46czNjcmV0LWNvdWNo", "Content-Type": "application/json"} // admin:s3cret-couch
		u := t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/var/lib/couchdb:rw,mode=1777", "--tmpfs", "/tmp:rw,mode=1777",
			"-e", "COUCHDB_USER=admin", "-e", "COUCHDB_PASSWORD=s3cret-couch", "-e", "COUCHDB_ERLANG_COOKIE=smoke-cookie", "-p", "127.0.0.1:15984:5984"})
		t.waitFor(90, time.Second, name, "couchdb did not become ready", func() bool {
			if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
				t.Fatalf("container exited early\n%s", t.Logs(name))
			}
			c, _ := Get(base + "/")
			return c == 200
		})
		_, welcome := Get(base + "/")
		t.Contains(welcome, `"couchdb":"Welcome"`, "welcome JSON")
		if t.Want != "" {
			t.Contains(welcome, `"version":"`+t.Want+`"`, "couchdb version")
		}
		for _, db := range []string{"_users", "_replicator", "_global_changes"} {
			if c, b := DoH("PUT", base+"/"+db, "", auth); c != 201 && c != 202 && c != 412 {
				t.Fatalf("system db %s PUT: %d %s", db, c, b)
			}
		}
		_, up := DoH("GET", base+"/_up", "", auth)
		t.Contains(up, `"status":"ok"`, "_up")
		put := func(path, body string) {
			if c, b := DoH("PUT", base+path, body, auth); c/100 != 2 {
				t.Fatalf("PUT %s: %d %s", path, c, b)
			}
		}
		put("/testdb", "")
		put("/testdb/doc1", `{"name":"quench","n":42}`)
		_, got := DoH("GET", base+"/testdb/doc1", "", auth)
		t.Contains(got, `"name":"quench"`, "document roundtrip")
		for _, n := range []string{"1", "2", "3"} {
			put("/testdb/k"+n, `{"v":`+n+`}`)
		}
		// querying the view starts the QuickJS view server (couchjs) and runs the map
		put("/testdb/_design/d", `{"views":{"byv":{"map":"function(doc){ if(doc.v){ emit(doc.v, doc.v*2); } }"}}}`)
		_, view := DoH("GET", base+"/testdb/_design/d/_view/byv", "", auth)
		t.Contains(view, `"key":1,"value":2`, "JS view rows")
		t.Logf("smoke test passed (couchdb %s: read-only rootfs, system DBs, document roundtrip, QuickJS view; user %s)", t.Want, u)
	})
}
