package smoke

import (
	"encoding/base64"
	"strings"
	"time"
)

func init() {
	// On a read-only rootfs with its state and file trees on volumes: the default admin gets an
	// API token, creates a user, the user uploads a file over the REST API and reads it back, a
	// wrong password is refused, and the file and the user survive a restart on the same volumes.
	register("sftpgo", func(t *T) {
		const base = "http://127.0.0.1:18080"
		const adminPW = "Smoke-Admin-2026"
		const userPW = "Smoke-User-2026"
		u := t.User("1001")
		t.Contains(t.RunAny("-v"), "SFTPGo "+t.Want, "sftpgo -v")
		state, files := t.Name("state"), t.Name("files")
		for _, v := range []string{state, files} {
			v := v
			t.Docker("volume", "create", v)
			t.Cleanup(func() { docker("volume", "rm", v) })
		}
		basic := func(user, pw string) map[string]string {
			return map[string]string{"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pw))}
		}
		bearer := func(tok string) map[string]string { return map[string]string{"Authorization": "Bearer " + tok} }
		start := func(name string) {
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", state + ":/var/lib/sftpgo", "-v", files + ":/srv/sftpgo",
				"-p", "127.0.0.1:18080:8080", "-e", "SFTPGO_DATA_PROVIDER__CREATE_DEFAULT_ADMIN=true",
				"-e", "SFTPGO_DEFAULT_ADMIN_USERNAME=admin", "-e", "SFTPGO_DEFAULT_ADMIN_PASSWORD=" + adminPW})
			t.waitFor(60, time.Second, name, "sftpgo never answered /healthz", func() bool {
				if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
					t.Fatalf("sftpgo exited\n%s", t.Logs(name))
				}
				c, _ := Get(base + "/healthz")
				return c == 200
			})
		}
		token := func(path, user, pw string) string {
			c, b := DoH("GET", base+path, "", basic(user, pw))
			if c != 200 {
				t.Fatalf("%s for %s answered %d: %s", path, user, c, b)
			}
			return t.Match(b, `"access_token":\s*"([^"]+)"`, path+" access token")
		}
		name := t.Name("")
		start(name)
		adm := token("/api/v2/token", "admin", adminPW)
		hdr := bearer(adm)
		hdr["Content-Type"] = "application/json"
		if c, b := DoH("POST", base+"/api/v2/users", `{"username":"alice","password":"`+userPW+`","status":1,"permissions":{"/":["*"]}}`, hdr); c != 201 {
			t.Fatalf("user create answered %d: %s\n%s", c, b, t.Logs(name))
		}
		usr := token("/api/v2/user/token", "alice", userPW)
		body := "quench sftpgo smoke " + t.Name("payload")
		up := bearer(usr)
		up["Content-Type"] = "application/octet-stream"
		if c, b := DoH("POST", base+"/api/v2/user/files/upload?path=hello.txt", body, up); c != 201 {
			t.Fatalf("upload answered %d: %s\n%s", c, b, t.Logs(name))
		}
		if c, b := DoH("GET", base+"/api/v2/user/files?path=hello.txt", "", bearer(usr)); c != 200 || b != body {
			t.Fatalf("download answered %d %q", c, b)
		}
		if c, _ := DoH("GET", base+"/api/v2/user/token", "", basic("alice", "wrong-password-1")); c != 401 {
			t.Fatalf("a wrong password answered %d, expected 401", c)
		}
		docker("rm", "-f", name)
		start(t.Name("again"))
		usr = token("/api/v2/user/token", "alice", userPW)
		if c, b := DoH("GET", base+"/api/v2/user/files?path=hello.txt", "", bearer(usr)); c != 200 || !strings.Contains(b, "quench sftpgo smoke") {
			t.Fatalf("the file is gone after a restart: %d %q", c, b)
		}
		t.Logf("smoke test passed (sftpgo %s: read-only boot, admin token, user created, REST upload and download, wrong password refused, user and file kept across a restart; user %s)", t.Want, u)
	})
}
