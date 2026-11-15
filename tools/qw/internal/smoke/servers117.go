package smoke

import (
	"bufio"
	"net"
	"strings"
	"time"
)

// ircLogin sends PASS, NICK and USER, then extra lines after the 001 welcome, and returns every line
// until one contains want (or the connection ends).
func ircLogin(addr, user, pass string, after []string, want string) (string, bool) {
	c, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return err.Error(), false
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(20 * time.Second))
	_, _ = c.Write([]byte("PASS " + pass + "\r\nNICK " + user + "\r\nUSER " + user + " 0 * :" + user + "\r\n"))
	r := bufio.NewReader(c)
	var seen strings.Builder
	for {
		l, err := r.ReadString('\n')
		if err != nil {
			return seen.String(), false
		}
		seen.WriteString(l)
		if strings.HasPrefix(l, "PING ") {
			_, _ = c.Write([]byte("PONG " + strings.TrimPrefix(l, "PING ")))
		}
		if strings.Contains(l, " 001 ") {
			for _, a := range after {
				_, _ = c.Write([]byte(a + "\r\n"))
			}
		}
		if strings.Contains(l, want) {
			return seen.String(), true
		}
	}
}

func init() {
	// On a read-only rootfs with /var/lib/soju on a volume: sojudb creates an admin from a password on
	// stdin, the user signs in and talks to BouncerServ, a wrong password is refused, and the user is
	// still there after a restart.
	register("soju", func(t *T) {
		const addr = "127.0.0.1:16668"
		const pw = "Smoke-Pass-2026"
		u := t.User("1001")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		flags := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/var/lib/soju"}
		if out, rc := t.RunAnyFor(2*time.Minute, append(flags, "--entrypoint", "/bin/sh"), "-c", "echo "+pw+" | sojudb create-user smoke -admin"); rc != 0 {
			t.Fatalf("sojudb create-user failed (%d):\n%s", rc, out)
		}
		start := func(name string) {
			t.Start(name, append(flags, "-p", addr+":6667"))
			t.waitFor(60, time.Second, name, "soju never answered", func() bool {
				if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
					t.Fatalf("soju exited\n%s", t.Logs(name))
				}
				c, err := net.DialTimeout("tcp", addr, time.Second)
				if err == nil {
					c.Close()
				}
				return err == nil
			})
		}
		name := t.Name("")
		start(name)
		if out, ok := ircLogin(addr, "smoke", pw, []string{"PRIVMSG BouncerServ :help"}, "BouncerServ"); !ok {
			t.Fatalf("sign-in or BouncerServ failed:\n%s\n%s", out, t.Logs(name))
		}
		docker("rm", "-f", name)
		start(t.Name("again"))
		if out, ok := ircLogin(addr, "smoke", "wrong-password-1", nil, " 001 "); ok {
			t.Fatalf("a wrong password signed in:\n%s", out)
		}
		if out, ok := ircLogin(addr, "smoke", pw, nil, " 001 "); !ok {
			t.Fatalf("sign-in after a restart failed (the user is gone?):\n%s", out)
		}
		t.Logf("smoke test passed (soju %s: read-only boot, admin created by sojudb from stdin, sign-in and BouncerServ, wrong password refused, user kept across a restart; user %s)", t.Want, u)
	})
}
