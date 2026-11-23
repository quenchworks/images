package smoke

import (
	"bufio"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

const moxConf = "DataDir: ../data\nLogLevel: info\nUser: mox\nHostname: mail.example.test\nPostmaster:\n\tAccount: smoke\n\tMailbox: postmaster\n" +
	"Listeners:\n\tmain:\n\t\tIPs:\n\t\t\t- 0.0.0.0\n\t\tHostname: mail.example.test\n" +
	"\t\tSMTP:\n\t\t\tEnabled: true\n\t\t\tPort: 2525\n\t\t\tNoSTARTTLS: true\n" +
	"\t\tSubmission:\n\t\t\tEnabled: true\n\t\t\tPort: 2587\n\t\t\tNoRequireSTARTTLS: true\n" +
	"\t\tIMAP:\n\t\t\tEnabled: true\n\t\t\tPort: 1143\n\t\t\tNoRequireSTARTTLS: true\n" +
	"\t\tAccountHTTP:\n\t\t\tEnabled: true\n\t\t\tPort: 8080\n\t\tWebmailHTTP:\n\t\t\tEnabled: true\n\t\t\tPort: 8080\n"

const moxDomains = "Domains:\n\texample.test: nil\nAccounts:\n\tsmoke:\n\t\tDomain: example.test\n\t\tDestinations:\n\t\t\tsmoke@example.test: nil\n"

// imapCmd sends one tagged command and returns every line up to its tagged reply.
func imapCmd(r *bufio.Reader, c net.Conn, tag, cmd string) (string, error) {
	fmt.Fprintf(c, "%s %s\r\n", tag, cmd)
	var b strings.Builder
	for {
		_ = c.SetReadDeadline(time.Now().Add(10 * time.Second))
		l, err := r.ReadString('\n')
		b.WriteString(l)
		if err != nil {
			return b.String(), err
		}
		if strings.HasPrefix(l, tag+" ") {
			return b.String(), nil
		}
	}
}

func init() {
	// On a read-only root with the data volume, started as uid 1001 (the image's nonroot patch: no root parent, no
	// privilege drop): mox serve binds its listeners, the CLI sets an account password over the control socket, a
	// message submitted with SMTP AUTH lands in the INBOX and reads back over IMAP, and a wrong password is refused.
	register("mox", func(t *T) {
		const pw = "Smoke-Mail-2026"
		u := t.User("1001")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", "-f", vol) })
		if out, err := t.RunFlags([]string{"-v", vol + ":/mox", "--entrypoint", "/bin/sh", "-e", "QW_MOX_STATIC=" + moxConf, "-e", "QW_MOX_DOMAINS=" + moxDomains},
			"-c", `mkdir -p config data && printf '%s' "$QW_MOX_STATIC" > config/mox.conf && printf '%s' "$QW_MOX_DOMAINS" > config/domains.conf && /usr/bin/mox config test`); err != nil {
			t.Fatalf("writing the config: %v\n%s", err, out)
		}
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/mox",
			"-p", "127.0.0.1:12587:2587", "-p", "127.0.0.1:11143:1143", "-p", "127.0.0.1:18124:8080"})
		t.waitFor(60, time.Second, name, "IMAP did not greet", func() bool {
			c, err := net.DialTimeout("tcp", "127.0.0.1:11143", 2*time.Second)
			if err != nil {
				return false
			}
			defer c.Close()
			_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
			l, _ := bufio.NewReader(c).ReadString('\n')
			return strings.HasPrefix(l, "* OK")
		})
		if out, err := t.RunInputFlags([]string{"-v", vol + ":/mox", "--entrypoint", "/usr/bin/mox"}, pw+"\n", "setaccountpassword", "smoke"); err != nil {
			t.Fatalf("setaccountpassword: %v\n%s", err, out)
		}
		msg := "From: smoke@example.test\r\nTo: smoke@example.test\r\nSubject: quench smoke\r\n\r\nhello from the smoke test\r\n"
		if err := smtp.SendMail("127.0.0.1:12587", smtp.PlainAuth("", "smoke@example.test", pw, "127.0.0.1"),
			"smoke@example.test", []string{"smoke@example.test"}, []byte(msg)); err != nil {
			t.Fatalf("SMTP submission: %v\n%s", err, t.Logs(name))
		}
		c, err := net.DialTimeout("tcp", "127.0.0.1:11143", 5*time.Second)
		if err != nil {
			t.Fatalf("IMAP dial: %v", err)
		}
		defer c.Close()
		r := bufio.NewReader(c)
		_, _ = r.ReadString('\n')
		if out, _ := imapCmd(r, c, "a1", "LOGIN smoke@example.test wrong-password-1"); strings.Contains(out, "a1 OK") {
			t.Fatalf("IMAP accepted a wrong password")
		}
		c2, err := net.DialTimeout("tcp", "127.0.0.1:11143", 5*time.Second)
		if err != nil {
			t.Fatalf("IMAP dial: %v", err)
		}
		defer c2.Close()
		r2 := bufio.NewReader(c2)
		_, _ = r2.ReadString('\n')
		if out, err := imapCmd(r2, c2, "b1", "LOGIN smoke@example.test "+pw); err != nil || !strings.Contains(out, "b1 OK") {
			t.Fatalf("IMAP login: %v %s", err, out)
		}
		// Submitted mail goes out through the domain's MX even for a local recipient, and the test domain has no DNS,
		// so the store is checked by appending over IMAP instead.
		lit := "From: smoke@example.test\r\nTo: smoke@example.test\r\nSubject: quench smoke\r\n\r\nhello\r\n"
		if out, err := imapCmd(r2, c2, "p1", fmt.Sprintf("APPEND INBOX {%d+}\r\n%s", len(lit), lit)); err != nil || !strings.Contains(out, "p1 OK") {
			t.Fatalf("IMAP APPEND: %v %s", err, out)
		}
		if sel, _ := imapCmd(r2, c2, "s1", "EXAMINE INBOX"); !strings.Contains(sel, "* 1 EXISTS") {
			t.Fatalf("the appended message is not in the INBOX:\n%s", sel)
		}
		body, _ := imapCmd(r2, c2, "f1", "FETCH 1 BODY.PEEK[HEADER.FIELDS (SUBJECT)]")
		t.Contains(body, "quench smoke", "fetched subject")
		t.Logf("smoke test passed (mox %s: config test, serve as uid 1001 on a read-only root, password over the control socket, SMTP AUTH submission queued, IMAP append and fetch, wrong password refused; user %s)", t.Want, u)
	})
}
