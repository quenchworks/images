package smoke

import (
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var sioSID = regexp.MustCompile(`"sid":"([^"]+)"`)

// loungeLogin opens a socket.io session over HTTP long-polling (EIO 4), joins the default namespace,
// sends auth:perform and returns everything the server answered after it.
func loungeLogin(base, user, pass string) (string, error) {
	url := base + "/socket.io/?EIO=4&transport=polling"
	c := &http.Client{Timeout: 30 * time.Second}
	get := func(u string) (string, error) {
		r, err := c.Get(u)
		if err != nil {
			return "", err
		}
		defer r.Body.Close()
		b, err := io.ReadAll(r.Body)
		return string(b), err
	}
	post := func(u, body string) error {
		r, err := c.Post(u, "text/plain;charset=UTF-8", strings.NewReader(body))
		if err != nil {
			return err
		}
		r.Body.Close()
		return nil
	}
	open, err := get(url)
	if err != nil {
		return "", err
	}
	m := sioSID.FindStringSubmatch(open)
	if m == nil {
		return open, io.ErrUnexpectedEOF
	}
	url += "&sid=" + m[1]
	if err := post(url, "40"); err != nil {
		return "", err
	}
	if _, err := get(url); err != nil { // 40{sid} and auth:start
		return "", err
	}
	if err := post(url, `42["auth:perform",{"user":"`+user+`","password":"`+pass+`"}]`); err != nil {
		return "", err
	}
	return get(url)
}

func init() {
	// On a read-only rootfs with THELOUNGE_HOME on a volume: the first start writes config.js, `thelounge
	// add` creates a user, and after a restart that user signs in over socket.io while a wrong password
	// gets auth:failed.
	register("thelounge", func(t *T) {
		const base = "http://127.0.0.1:19000"
		const pw = "Smoke-Pass-2026"
		u := t.User("1001")
		t.Contains(t.RunAny("--version"), t.Want, "thelounge --version")
		vol := t.Name("home")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		flags := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/var/opt/thelounge"}
		start := func(name string) {
			t.Start(name, append(flags, "-p", "127.0.0.1:19000:9000"))
			body := t.WaitHTTP(base+"/", 60, name)
			t.Contains(body, "The Lounge", "the web UI")
		}
		name := t.Name("")
		start(name)
		if out, rc := t.RunAnyFor(2*time.Minute, flags, "add", "smoke", "--password", pw); rc != 0 {
			t.Fatalf("thelounge add failed (%d):\n%s", rc, out)
		}
		docker("rm", "-f", name)
		start(t.Name("again"))
		if out, err := loungeLogin(base, "smoke", "wrong-password-1"); err != nil || !strings.Contains(out, "auth:failed") {
			t.Fatalf("a wrong password was not refused: %v %q", err, out)
		}
		if out, err := loungeLogin(base, "smoke", pw); err != nil || !strings.Contains(out, `"configuration"`) || strings.Contains(out, "auth:failed") {
			t.Fatalf("sign-in failed: %v %q", err, out)
		}
		t.Logf("smoke test passed (thelounge %s: read-only boot, user added on the volume, sign-in over socket.io after a restart, wrong password refused; user %s)", t.Want, u)
	})
}
