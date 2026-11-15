package smoke

import (
	"bufio"
	"net"
	"strings"
	"time"
)

// ircSession connects, registers nick, sends cmds after the 001 welcome and returns every line until
// one contains want (or the deadline passes).
func ircSession(addr, nick string, cmds []string, want string) (string, bool) {
	c, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return err.Error(), false
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(20 * time.Second))
	_, _ = c.Write([]byte("NICK " + nick + "\r\nUSER " + nick + " 0 * :" + nick + "\r\n"))
	r := bufio.NewReader(c)
	var seen strings.Builder
	for {
		l, err := r.ReadString('\n')
		if err != nil {
			return seen.String(), false
		}
		seen.WriteString(l)
		switch {
		case strings.HasPrefix(l, "PING "):
			_, _ = c.Write([]byte("PONG " + strings.TrimPrefix(l, "PING ")))
		case strings.Contains(l, " 001 "):
			for _, cmd := range cmds {
				_, _ = c.Write([]byte(cmd + "\r\n"))
			}
		}
		if want != "" && strings.Contains(l, want) {
			return seen.String(), true
		}
		if want == "" && strings.Contains(l, " 001 ") {
			return seen.String(), true
		}
	}
}

func init() {
	// On a read-only rootfs with /ircd on a volume: a client gets the 001 welcome, registers its nick with
	// NickServ, and after a restart on the same volume identifies to that account; a wrong password fails.
	register("ergo", func(t *T) {
		const addr = "127.0.0.1:16667"
		const pw = "Smoke-Pass-2026"
		u := t.User("1001")
		t.Contains(t.RunAny("--version"), t.Want, "ergo --version")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		start := func(name string) {
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/ircd", "-p", addr + ":6667"})
			t.waitFor(60, time.Second, name, "ergo never welcomed a client", func() bool {
				if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
					t.Fatalf("ergo exited\n%s", t.Logs(name))
				}
				_, ok := ircSession(addr, "probe", nil, "")
				return ok
			})
		}
		name := t.Name("")
		start(name)
		if out, ok := ircSession(addr, "smoke", []string{"PRIVMSG NickServ :REGISTER " + pw}, "Account created"); !ok {
			t.Fatalf("NickServ REGISTER failed:\n%s\n%s", out, t.Logs(name))
		}
		docker("rm", "-f", name)
		start(t.Name("again"))
		if out, ok := ircSession(addr, "smoke2", []string{"PRIVMSG NickServ :IDENTIFY smoke wrong-password-1"}, "logged in"); ok {
			t.Fatalf("a wrong password identified:\n%s", out)
		}
		if out, ok := ircSession(addr, "smoke3", []string{"PRIVMSG NickServ :IDENTIFY smoke " + pw}, "logged in"); !ok {
			t.Fatalf("IDENTIFY after a restart failed (the account is gone?):\n%s", out)
		}
		t.Logf("smoke test passed (ergo %s: read-only boot, IRC welcome, NickServ account registered and identified after a restart, wrong password refused; user %s)", t.Want, u)
	})
}
