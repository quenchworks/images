package smoke

import (
	"encoding/base64"
	"strings"
	"time"
)

func init() {
	// On a read-only rootfs with /data on a volume and one read-write account: an anonymous
	// request is refused, the account uploads a file with HTTP PUT and reads it back, a wrong
	// password is refused, and the file is still there after a restart on the same volume.
	register("dufs", func(t *T) {
		const base = "http://127.0.0.1:15000"
		const pw = "Smoke-Pass-2026"
		u := t.User("1001")
		t.Contains(t.RunAny("--version"), t.Want, "dufs --version")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		auth := func(p string) map[string]string {
			return map[string]string{"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte("smoke:"+p))}
		}
		// an empty named volume is seeded from the image's /data, which apko makes 1001-owned
		start := func(name string) {
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/data", "-p", "127.0.0.1:15000:5000",
				"-e", "DUFS_AUTH=smoke:" + pw + "@/:rw", "-e", "DUFS_ALLOW_UPLOAD=true"})
			t.waitFor(30, time.Second, name, "dufs never answered", func() bool {
				if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
					t.Fatalf("dufs exited\n%s", t.Logs(name))
				}
				c, _ := Get(base + "/")
				return c == 401
			})
		}
		name := t.Name("")
		start(name)
		body := "quench dufs smoke " + t.Name("payload")
		if c, b := DoH("PUT", base+"/hello.txt", body, auth(pw)); c != 201 {
			t.Fatalf("upload answered %d %s\n%s", c, b, t.Logs(name))
		}
		if c, b := DoH("GET", base+"/hello.txt", "", auth(pw)); c != 200 || b != body {
			t.Fatalf("download answered %d %q", c, b)
		}
		if c, _ := DoH("GET", base+"/hello.txt", "", auth("wrong-password-1")); c != 401 {
			t.Fatalf("a wrong password answered %d, expected 401", c)
		}
		docker("rm", "-f", name)
		start(t.Name("again"))
		if c, b := DoH("GET", base+"/hello.txt", "", auth(pw)); c != 200 || !strings.Contains(b, "quench dufs smoke") {
			t.Fatalf("the file is gone after a restart: %d %q", c, b)
		}
		t.Logf("smoke test passed (dufs %s: read-only boot, anonymous refused, upload and download with an account, wrong password refused, kept across a restart; user %s)", t.Want, u)
	})
}
