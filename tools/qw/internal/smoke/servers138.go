package smoke

import (
	"io"
	"net/http"
	"strings"
	"time"
)

func baikalDo(method, url, user, pass, body string, hdr map[string]string) (int, string) {
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	req.SetBasicAuth(user, pass)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err.Error()
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return r.StatusCode, string(b)
}

func init() {
	// On a read-only rootfs with /var/lib/baikal on a volume: the setup step writes baikal.yaml, the SQLite
	// schema and a DAV user (with its default calendar) from the environment; CalDAV refuses a wrong
	// password, answers PROPFIND on the calendar, stores an event and returns it after a restart.
	register("baikal", func(t *T) {
		const base = "http://127.0.0.1:18090"
		const user, pw = "smoke", "Smoke-Baikal-2026"
		const cal = base + "/dav.php/calendars/smoke/default/"
		const event = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//QuenchWorks//smoke//EN\r\nBEGIN:VEVENT\r\nUID:quench-smoke-baikal\r\nDTSTAMP:20261116T000000Z\r\nDTSTART:20261117T100000Z\r\nDTEND:20261117T110000Z\r\nSUMMARY:Quench smoke\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
		u := t.User("1001")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		flags := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/var/lib/baikal", "-p", "127.0.0.1:18090:8080",
			"-e", "BAIKAL_ADMIN_PASSWORD=Smoke-Admin-2026", "-e", "BAIKAL_DAV_AUTH_TYPE=Basic",
			"-e", "BAIKAL_DAV_USER=" + user, "-e", "BAIKAL_DAV_PASSWORD=" + pw}
		propfind := map[string]string{"Depth": "0", "Content-Type": "application/xml"}
		start := func(name string) {
			t.Start(name, flags)
			for i := 0; i < 90; i++ {
				if c, _ := baikalDo("PROPFIND", cal, user, pw, "", propfind); c == 207 {
					return
				}
				time.Sleep(time.Second)
			}
			t.Fatalf("baikal did not answer PROPFIND on %s:\n%s", cal, t.Logs(name))
		}
		name := t.Name("")
		start(name)
		if c, _ := baikalDo("PROPFIND", cal, user, "wrong-password-1", "", propfind); c != http.StatusUnauthorized {
			t.Fatalf("a wrong password got %d, want 401", c)
		}
		if c, body := baikalDo("PUT", cal+"smoke.ics", user, pw, event, map[string]string{"Content-Type": "text/calendar"}); c != http.StatusCreated && c != http.StatusNoContent {
			t.Fatalf("PUT event: %d %s", c, body)
		}
		docker("rm", "-f", name)
		start(t.Name("again"))
		if c, body := baikalDo("GET", cal+"smoke.ics", user, pw, "", nil); c != http.StatusOK || !strings.Contains(body, "quench-smoke-baikal") {
			t.Fatalf("GET event after a restart: %d %q", c, body)
		}
		t.Logf("smoke test passed (baikal %s: read-only boot, config, schema and DAV user from the environment, wrong password refused, CalDAV event stored and read back after a restart; user %s)", t.Want, u)
	})
}
