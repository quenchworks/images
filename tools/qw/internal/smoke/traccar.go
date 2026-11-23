package smoke

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

func init() {
	// On a read-only rootfs with the H2 database on a volume: the server boots, migrates and reports its version,
	// the web UI is served, the first registered user becomes admin and a wrong password is refused, a device sends
	// a position over the OsmAnd protocol (HTTP on 5055) and the API reads it back, and after a restart on the same
	// volume the position is still there. Logs must show no Exception.
	register("traccar", func(t *T) {
		const base, osm = "http://127.0.0.1:18083", "http://127.0.0.1:15056"
		const email, pw = "admin@example.com", "Smoke-Traccar-2026"
		u := t.User("1001")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		flags := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--tmpfs", "/opt/traccar/logs:rw,mode=1777", "--tmpfs", "/opt/traccar/override:rw,mode=1777",
			"-v", vol + ":/opt/traccar/data", "-p", "127.0.0.1:18083:8082", "-p", "127.0.0.1:15056:5055"}
		call := func(method, path, user, pass, body string) (int, string) {
			req, _ := http.NewRequest(method, base+path, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			if user != "" {
				req.SetBasicAuth(user, pass)
			}
			return insecureDo(req)
		}
		name := t.Name("")
		t.Start(name, flags)
		t.waitCode(base+"/api/server", 80, 3*time.Second, name, 200)
		_, srv := call("GET", "/api/server", "", "", "")
		t.Contains(srv, `"version":"`+t.Want, "the server version")
		_, page := call("GET", "/", "", "", "")
		t.ContainsFold(page, "<html", "the web UI")
		if c, b := call("POST", "/api/users", "", "", fmt.Sprintf(`{"name":"admin","email":%q,"password":%q}`, email, pw)); c != 200 {
			t.Fatalf("register: %d %s", c, b)
		}
		if c, _ := call("GET", "/api/devices", email, "wrong-password-1", ""); c != 401 {
			t.Fatalf("a wrong password got %d, want 401", c)
		}
		if c, b := call("POST", "/api/devices", email, pw, `{"name":"smoke","uniqueId":"424242"}`); c != 200 {
			t.Fatalf("add device: %d %s", c, b)
		}
		if c, _ := Get(fmt.Sprintf("%s/?id=424242&lat=31.95&lon=35.93&speed=10&timestamp=%d", osm, time.Now().Unix())); c != 200 {
			t.Fatalf("OsmAnd position: %d", c)
		}
		pos := func() string {
			for i := 0; i < 20; i++ {
				if _, b := call("GET", "/api/devices", email, pw, ""); strings.Contains(b, `"positionId":`) && !strings.Contains(b, `"positionId":0`) {
					_, p := call("GET", "/api/positions?id="+strings.SplitN(strings.SplitN(b, `"positionId":`, 2)[1], ",", 2)[0], email, pw, "")
					return p
				}
				time.Sleep(time.Second)
			}
			return ""
		}
		t.Contains(pos(), `"latitude":31.95`, "the stored position")
		docker("stop", "-t", "30", name)
		if logs := t.Logs(name); strings.Contains(logs, "Exception") {
			t.Fatalf("exception in the logs:\n%s", logs)
		}
		docker("rm", "-f", name)
		name = t.Name("again")
		t.Start(name, flags)
		t.waitCode(base+"/api/server", 80, 3*time.Second, name, 200)
		t.Contains(pos(), `"longitude":35.93`, "the position after a restart")
		t.Logf("smoke test passed (traccar %s: read-only boot, version API, web UI, first user admin, wrong password 401, OsmAnd position stored and read back, kept across a restart; user %s)", t.Want, u)
	})
}
