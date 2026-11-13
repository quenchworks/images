package smoke

import (
	"encoding/json"
	"net"
	"regexp"
	"strings"
	"time"
)

func init() {
	register("openldap", func(t *T) {
		const base, bindDN, bindPW = "dc=my-domain,dc=com", "cn=Manager,dc=my-domain,dc=com", "secret" // the image's default slapd.conf
		name := t.Name("")
		t.Start(name, nil)
		ldap := []string{"-x", "-H", "ldap://127.0.0.1:1389"}
		ex := func(cmd string, args ...string) (string, error) {
			return docker(append(append([]string{"exec", name, cmd}, ldap...), args...)...)
		}
		t.waitFor(30, time.Second, name, "slapd never answered on 1389", func() bool { _, err := ex("ldapsearch", "-b", "", "-s", "base", "-LLL"); return err == nil })
		who, _ := ex("ldapwhoami", "-D", bindDN, "-w", bindPW)
		if !strings.EqualFold(strings.TrimSpace(who), "dn:"+bindDN) {
			t.Fatalf("unexpected whoami: %s", who)
		}
		ldif := "dn: " + base + "\nobjectClass: dcObject\nobjectClass: organization\ndc: my-domain\no: QuenchWorks Smoke Test\n\ndn: cn=smoke," + base + "\nobjectClass: organizationalRole\ncn: smoke\n"
		if out, err := runHostIn(ldif, "docker", append(append([]string{"exec", "-i", name, "ldapadd"}, ldap...), "-D", bindDN, "-w", bindPW)...); err != nil {
			t.Fatalf("ldapadd: %s", out)
		}
		out, _ := ex("ldapsearch", "-b", base, "-LLL", "(cn=smoke)", "dn")
		t.ContainsFold(out, "cn=smoke,"+base, "search returned the seeded entry")
		t.Match(t.Docker("exec", name, "slappasswd", "-s", "hunter2"), `^\{SSHA\}`, "slappasswd")
		vv, _ := docker("exec", name, "slapd", "-VV")
		v := regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+`).FindString(vv)
		if v == "" || (t.Want != "" && v != t.Want) {
			t.Fatalf("slapd version %q, expected %s", v, t.Want)
		}
		t.Logf("smoke test passed (openldap %s, bind+add+search on 1389; user %s)", v, t.Docker("inspect", t.Image, "--format", "{{.Config.User}}"))
	})
	register("owncast", func(t *T) {
		const b = "http://127.0.0.1:18080"
		u := t.Docker("inspect", t.Image, "--format", "{{.Config.User}}")
		net_, name, src, vol := t.Name("net"), t.Name(""), t.Name("src"), t.Name("data")
		pw, sk := t.Name("admin"), t.Name("key")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		t.Start(name, []string{"--network", net_, "--network-alias", "owncast", "--read-only", "--tmpfs", "/tmp:uid=1001,gid=1001", "-v", vol + ":/app/data", "-p", "127.0.0.1:18080:8080"},
			"-adminpassword", pw, "-streamkey", sk)
		st := t.WaitHTTP(b+"/api/status", 60, name)
		var s struct {
			VersionNumber string
			Online        bool
		}
		json.Unmarshal([]byte(st), &s)
		if s.VersionNumber == "" || (t.Want != "" && s.VersionNumber != t.Want) {
			t.Fatalf("owncast %q, expected %s", s.VersionNumber, t.Want)
		}
		if c, _ := Get("http://admin:wrong@127.0.0.1:18080/api/admin/status"); c != 401 {
			t.Fatalf("a wrong admin password got HTTP %d", c)
		}
		if c, _ := Get("http://admin:" + pw + "@127.0.0.1:18080/api/admin/status"); c != 200 {
			t.Fatalf("admin status with the password: HTTP %d", c)
		}
		t.Start(src, []string{"--network", net_, "--entrypoint", "/usr/bin/ffmpeg"}, "-hide_banner", "-loglevel", "error", "-re",
			"-f", "lavfi", "-i", "testsrc=size=640x360:rate=25", "-f", "lavfi", "-i", "sine=frequency=440", "-t", "120",
			"-c:v", "libx264", "-preset", "veryfast", "-g", "50", "-c:a", "aac", "-f", "flv", "rtmp://owncast:1935/live/"+sk)
		t.waitFor(60, time.Second, name, "stream never went online", func() bool {
			_, st := Get(b + "/api/status")
			json.Unmarshal([]byte(st), &s)
			return s.Online
		})
		seg := regexp.MustCompile(`\.ts|\.m4s`)
		t.waitFor(60, time.Second, name, "no HLS segments", func() bool {
			_, pl := Get(b + "/hls/stream.m3u8")
			if !strings.HasPrefix(pl, "#EXTM3U") {
				return false
			}
			v := regexp.MustCompile(`(?m)^\S*\.m3u8$`).FindString(pl)
			if v == "" {
				return false
			}
			_, vp := Get(b + "/hls/" + v)
			return seg.MatchString(vp)
		})
		if regexp.MustCompile(`(?i)read-only file system|panic`).MatchString(t.Logs(name)) {
			t.Fatalf("crashed or wrote to the root\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed (owncast %s: admin auth, RTMP ingest, HLS segments; user %s)", s.VersionNumber, u)
	})
	register("pgpool", func(t *T) {
		t.Match(t.RunEntryAny("/usr/bin/pgpool", "--version"), `pgpool-II version [0-9]+\.[0-9]+\.[0-9]+`, "version")
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/var/run/pgpool:uid=1001,gid=1001", "--tmpfs", "/tmp:uid=1001,gid=1001", "-p", "127.0.0.1:9999:9999", "-p", "127.0.0.1:9898:9898"})
		// a v3 StartupMessage for user pgpool; a live pgpool answers R (auth) or E (error)
		startup := []byte("\x00\x00\x00\x15\x00\x03\x00\x00user\x00pgpool\x00\x00")
		var first byte
		t.waitFor(45, time.Second, name, "no PostgreSQL protocol reply on :9999", func() bool {
			if s := t.Docker("inspect", "-f", "{{.State.Status}}", name); s != "running" {
				t.Fatalf("container died during startup\n%s", t.Logs(name))
			}
			c, err := net.DialTimeout("tcp", "127.0.0.1:9999", 3*time.Second)
			if err != nil {
				return false
			}
			defer c.Close()
			c.SetDeadline(time.Now().Add(10 * time.Second))
			c.Write(startup)
			b := make([]byte, 1)
			if n, _ := c.Read(b); n == 1 {
				first = b[0]
			}
			return first == 'R' || first == 'E'
		})
		t.WaitTCP(10, name, "127.0.0.1:9898")
		t.Logf("smoke test passed (v3 handshake answered %q, PCP listening; nonroot user: %s)", first, t.User("1001"))
	})
}
