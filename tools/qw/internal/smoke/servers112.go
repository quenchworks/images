package smoke

import (
	"bufio"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func init() {
	// On a read-only rootfs with a config (SMTP 2525, IMAP 1143, local mailboxes) and /data on a volume:
	// SMTP greets with 220, `maddy creds create` and `imap-acct create` make a user, IMAP logs that user
	// in, a wrong password is refused, and the account survives a restart on the same volume.
	register("maddy", func(t *T) {
		const pw = "Smoke-Mail-2026"
		u := t.User("1001")
		t.Contains(t.RunAny("version"), t.Want, "maddy version")
		dir, err := os.MkdirTemp(envOr("TMPDIR", ""), "maddy-smoke-")
		if err != nil {
			t.Fatalf("temp dir: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(dir) })
		_ = os.Chmod(dir, 0o755)
		cfg := `$(hostname) = mx.quench.test
$(primary_domain) = quench.test
$(local_domains) = $(primary_domain)
hostname $(hostname)
tls off
auth.pass_table local_authdb {
    table sql_table {
        driver sqlite3
        dsn credentials.db
        table_name passwords
    }
}
storage.imapsql local_mailboxes {
    driver sqlite3
    dsn imapsql.db
}
smtp tcp://0.0.0.0:2525 {
    deliver_to &local_mailboxes
}
imap tcp://0.0.0.0:1143 {
    insecure_auth yes
    auth &local_authdb
    storage &local_mailboxes
}
`
		if err := os.WriteFile(filepath.Join(dir, "maddy.conf"), []byte(cfg), 0o644); err != nil {
			t.Fatalf("%v", err)
		}
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		greet := func(addr, want string) bool {
			c, err := net.DialTimeout("tcp", addr, 2*time.Second)
			if err != nil {
				return false
			}
			defer c.Close()
			_ = c.SetDeadline(time.Now().Add(5 * time.Second))
			l, _ := bufio.NewReader(c).ReadString('\n')
			return strings.HasPrefix(l, want)
		}
		imapLogin := func(p string) string {
			c, err := net.DialTimeout("tcp", "127.0.0.1:11143", 3*time.Second)
			if err != nil {
				return err.Error()
			}
			defer c.Close()
			_ = c.SetDeadline(time.Now().Add(10 * time.Second))
			r := bufio.NewReader(c)
			_, _ = r.ReadString('\n')
			_, _ = c.Write([]byte("a1 LOGIN smoke@quench.test " + p + "\r\n"))
			for {
				l, err := r.ReadString('\n')
				if err != nil || strings.HasPrefix(l, "a1 ") {
					return l
				}
			}
		}
		start := func(name string) {
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/data", "-v", filepath.Join(dir, "maddy.conf") + ":/data/maddy.conf:ro",
				"-p", "127.0.0.1:12525:2525", "-p", "127.0.0.1:11143:1143"})
			t.waitFor(60, time.Second, name, "maddy never greeted on SMTP and IMAP", func() bool {
				if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
					t.Fatalf("maddy exited\n%s", t.Logs(name))
				}
				return greet("127.0.0.1:12525", "220") && greet("127.0.0.1:11143", "* OK")
			})
		}
		name := t.Name("")
		start(name)
		for _, args := range [][]string{{"creds", "create", "--password", pw, "smoke@quench.test"}, {"imap-acct", "create", "smoke@quench.test"}} {
			full := append([]string{"exec", name, "/usr/bin/maddy", "-config", "/data/maddy.conf"}, args...)
			if out, err := docker(full...); err != nil {
				t.Fatalf("maddy %s: %v\n%s", strings.Join(args, " "), err, out)
			}
		}
		if l := imapLogin(pw); !strings.Contains(l, "a1 OK") {
			t.Fatalf("IMAP login failed: %s\n%s", l, t.Logs(name))
		}
		if l := imapLogin("wrong-password-1"); strings.Contains(l, "a1 OK") {
			t.Fatalf("a wrong password logged in over IMAP: %s", l)
		}
		docker("rm", "-f", name)
		again := t.Name("again")
		start(again)
		if l := imapLogin(pw); !strings.Contains(l, "a1 OK") {
			t.Fatalf("the account is gone after a restart: %s", l)
		}
		t.Logf("smoke test passed (maddy %s: read-only boot, SMTP 220 and IMAP greeting, user created by the CLI, IMAP login, wrong password refused, kept across a restart; user %s)", t.Want, u)
	})
}
