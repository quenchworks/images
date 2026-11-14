package smoke

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/jpeg"
	"strings"
	"time"
)

func init() {
	// On a read-only rootfs: the first user becomes the admin and signs in, a valid JPEG upload
	// passes the EXIF strip (imaging.Decode), and the palette-overflow BMP declared as image/jpeg
	// does not take the server down (CVE-2023-36308 mitigation).
	register("memos", func(t *T) {
		const base = "http://127.0.0.1:15230"
		u := t.User("1001")
		t.Contains(t.RunAny("version"), t.Want, "memos version")
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--tmpfs", "/var/opt/memos:rw,uid=1001,gid=1001", "-p", "127.0.0.1:15230:5230"})
		t.waitFor(60, time.Second, name, "memos never served /", func() bool {
			if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
				t.Fatalf("memos exited\n%s", t.Logs(name))
			}
			c, _ := Get(base + "/")
			return c == 200
		})
		_, page := Get(base + "/")
		t.ContainsFold(page, "memos", "embedded web app")
		j := map[string]string{"Content-Type": "application/json"}
		if c, b := DoH("POST", base+"/api/users", `{"username":"smoke","password":"Smoke-Pass-2026"}`, j); c != 200 {
			t.Fatalf("create the first user: %d %s", c, b)
		}
		_, in := DoH("POST", base+"/api/auth/signin", `{"passwordCredentials":{"username":"smoke","password":"Smoke-Pass-2026"}}`, j)
		tok := t.Match(in, `"accessToken":"([^"]+)"`, "sign-in token")
		auth := map[string]string{"Content-Type": "application/json", "Authorization": "Bearer " + tok}
		upload := func(fname, typ string, b []byte) (int, string) {
			return DoH("POST", base+"/api/attachments", `{"filename":"`+fname+`","type":"`+typ+`","content":"`+base64.StdEncoding.EncodeToString(b)+`"}`, auth)
		}
		var good bytes.Buffer
		jpeg.Encode(&good, image.NewNRGBA(image.Rect(0, 0, 40, 30)), nil)
		if c, b := upload("good.jpg", "image/jpeg", good.Bytes()); c != 200 || !strings.Contains(b, `"name":"attachments/`) {
			t.Fatalf("valid JPEG upload: %d %s\n%s", c, b, t.Logs(name))
		}
		c, _ := upload("evil.jpg", "image/jpeg", paletteOverflowBMP())
		time.Sleep(2 * time.Second)
		if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
			t.Fatalf("memos died on the crafted image\n%s", t.Logs(name))
		}
		// Memos keeps the original when the EXIF strip fails to decode it, so a 200 is fine; the
		// strip ran imaging.Decode on it. Upstream 26.11.0 already survives too (x/image v0.45.0 has
		// the bmp fix); unlike Shiori and listmonk, this guards against a regression.
		if c, _ := Get(base + "/"); c != 200 {
			t.Fatalf("memos stopped serving after the crafted image")
		}
		t.Logf("smoke test passed (memos %s: first user + sign-in, read-only boot, JPEG upload, crafted BMP through the EXIF strip (HTTP %d) with the server up; user %s)", t.Want, c, u)
	})
}
