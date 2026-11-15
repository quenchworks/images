package smoke

import (
	"net/smtp"
	"strings"
	"time"
)

func init() {
	// On a read-only rootfs with file storage on a volume: the web UI answers, a mail sent over
	// SMTP shows up in the recipient's mailbox through the REST API with its subject and body,
	// and it is still there after a restart on the same volume.
	register("inbucket", func(t *T) {
		const web = "http://127.0.0.1:19000"
		const smtpAddr = "127.0.0.1:12500"
		u := t.User("1001")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		// an empty named volume is seeded from the image's /storage, which apko makes 1001-owned
		start := func(name string) {
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/storage",
				"-p", "127.0.0.1:19000:9000", "-p", "127.0.0.1:12500:2500"})
			t.waitFor(60, time.Second, name, "inbucket never served its web UI", func() bool {
				if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
					t.Fatalf("inbucket exited\n%s", t.Logs(name))
				}
				c, _ := Get(web + "/")
				return c == 200
			})
		}
		name := t.Name("")
		start(name)
		t.Contains(t.Logs(name), t.Want, "version in the startup log")
		subject := "quench smoke " + t.Name("subj")
		msg := "From: smoke@example.com\r\nTo: alice@example.com\r\nSubject: " + subject + "\r\n\r\nhello from the quench smoke test\r\n"
		if err := smtp.SendMail(smtpAddr, nil, "smoke@example.com", []string{"alice@example.com"}, []byte(msg)); err != nil {
			t.Fatalf("SMTP send: %v\n%s", err, t.Logs(name))
		}
		list := func() string {
			_, b := Get(web + "/api/v1/mailbox/alice")
			return b
		}
		t.waitFor(20, time.Second, name, "the mail never reached the mailbox", func() bool { return strings.Contains(list(), subject) })
		id := t.Match(list(), `"id":"([^"]+)"`, "message id in the mailbox list")
		_, body := Get(web + "/api/v1/mailbox/alice/" + id)
		t.Contains(body, "hello from the quench smoke test", "message body over REST")
		docker("rm", "-f", name)
		start(t.Name("again"))
		t.Contains(list(), subject, "mailbox after a restart on the same volume")
		t.Logf("smoke test passed (inbucket %s: read-only boot, mail sent over SMTP read back over REST, kept across a restart; user %s)", t.Want, u)
	})
}
