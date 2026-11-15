package smoke

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const smokeEvent = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//quench//smoke//EN\r\nBEGIN:VEVENT\r\nUID:quench-smoke-1\r\nDTSTAMP:20261115T120000Z\r\nDTSTART:20261116T090000Z\r\nDTEND:20261116T100000Z\r\nSUMMARY:quench smoke\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"

func init() {
	// On a read-only rootfs with collections on a volume and bcrypt htpasswd users: anonymous and wrong
	// passwords get 401, the user creates a calendar and stores an event, and the event reads back after a
	// restart on the same volume.
	register("radicale", func(t *T) {
		const base = "http://127.0.0.1:15232"
		const user, pw = "smoke", "Smoke-Pass-2026"
		u := t.User("1001")
		dir := t.TempDir()
		h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.MinCost)
		if err != nil {
			t.Fatalf("bcrypt: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "users"), []byte(user+":"+string(h)+"\n"), 0o644); err != nil {
			t.Fatalf("users: %v", err)
		}
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		start := func(name string) {
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/var/lib/radicale",
				"-v", filepath.Join(dir, "users") + ":/etc/radicale/users:ro", "-p", "127.0.0.1:15232:5232"})
			t.waitFor(60, time.Second, name, "radicale never answered", func() bool {
				if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
					t.Fatalf("radicale exited\n%s", t.Logs(name))
				}
				code, _ := restDo("PROPFIND", base+"/", "", "", "")
				return code == http.StatusUnauthorized
			})
		}
		name := t.Name("")
		start(name)
		if code, _ := restDo("PROPFIND", base+"/", user, "wrong-password-1", ""); code != http.StatusUnauthorized {
			t.Fatalf("a wrong password got %d, want 401", code)
		}
		cal := base + "/" + user + "/smoke-cal/"
		if code, body := restDo("MKCALENDAR", cal, user, pw, ""); code != http.StatusCreated {
			t.Fatalf("MKCALENDAR: %d %s\n%s", code, body, t.Logs(name))
		}
		if code, body := restDo("PUT", cal+"event.ics", user, pw, smokeEvent); code != http.StatusCreated {
			t.Fatalf("PUT event: %d %s\n%s", code, body, t.Logs(name))
		}
		docker("rm", "-f", name)
		start(t.Name("again"))
		if code, body := restDo("GET", cal+"event.ics", user, pw, ""); code != http.StatusOK || !strings.Contains(body, "UID:quench-smoke-1") {
			t.Fatalf("event after a restart: %d %q (the collection is gone?)", code, body)
		}
		t.Logf("smoke test passed (radicale %s: read-only boot, bcrypt htpasswd enforced, calendar created and an event kept across a restart; user %s)", t.Want, u)
	})
}

func init() {
	// On a read-only rootfs with /tmp writable: nginx serves the CyberChef page (with its version), the
	// main bundle and an operation module, and answers 404 for a missing path.
	register("cyberchef", func(t *T) {
		const base = "http://127.0.0.1:18081"
		u := t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-p", "127.0.0.1:18081:8080"})
		page := t.WaitHTTP(base+"/", 60, name)
		t.Contains(page, "<title>CyberChef</title>", "the page")
		t.Contains(page, t.Want, "the version in the page")
		for _, p := range []string{"/assets/main.js", "/modules/Bletchley.js"} {
			if code, _ := restDo("GET", base+p, "", "", ""); code != 200 {
				t.Fatalf("%s answered %d\n%s", p, code, t.Logs(name))
			}
		}
		if code, _ := restDo("GET", base+"/no-such-file", "", "", ""); code != 404 {
			t.Fatalf("a missing path answered %d", code)
		}
		t.Logf("smoke test passed (cyberchef %s: read-only boot, page, main bundle and an operation module served by nginx; user %s)", t.Want, u)
	})
}
