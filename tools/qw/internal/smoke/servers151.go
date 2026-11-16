package smoke

import (
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"time"
)

// psiDo sends a request (no redirects followed) and returns the status, body and response headers.
func psiDo(method, url, body string, hdr map[string]string) (int, string, http.Header) {
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	r, err := noRedirect.Do(req)
	if err != nil {
		return 0, err.Error(), nil
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return r.StatusCode, string(b), r.Header
}

func init() {
	// On a read-only rootfs with /data on a volume and an upload password: the upload page answers, an upload
	// without the password is refused, a tus upload with it stores a file that downloads byte for byte, the admin
	// data needs the admin password, and the file survives a restart.
	register("psitransfer", func(t *T) {
		const base = "http://127.0.0.1:18104"
		const upw, apw = "Smoke-Upload-2026!", "Smoke-Admin-2026!"
		const content = "quench psitransfer smoke file\n"
		u := t.User("1001")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		start := func(name string) {
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/data", "-p", "127.0.0.1:18104:3000",
				"-e", "PSITRANSFER_UPLOAD_PASS=" + upw, "-e", "PSITRANSFER_ADMIN_PASS=" + apw})
			t.WaitHTTP(base+"/", 60, name)
		}
		b64 := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
		create := func(pass string) (int, string) {
			h := map[string]string{"Tus-Resumable": "1.0.0", "Upload-Length": "30",
				"Upload-Metadata": "sid " + b64("quenchsmoke") + ",retention " + b64("604800") + ",name " + b64("smoke.txt")}
			if pass != "" {
				h["x-passwd"] = pass
			}
			c, _, hd := psiDo("POST", base+"/files", "", h)
			loc := ""
			if hd != nil {
				loc = hd.Get("Location")
			}
			return c, loc
		}
		name := t.Name("")
		start(name)
		if c, _ := create(""); c != http.StatusUnauthorized {
			t.Fatalf("an upload without the password answered %d, want 401", c)
		}
		c, loc := create(upw)
		if c != http.StatusCreated || loc == "" {
			t.Fatalf("create the upload: %d %q\n%s", c, loc, t.Logs(name))
		}
		if i := strings.Index(loc, "/files/"); i >= 0 {
			loc = loc[i:]
		}
		if c, b, _ := psiDo("PATCH", base+loc, content, map[string]string{"Tus-Resumable": "1.0.0", "Upload-Offset": "0",
			"Content-Type": "application/offset+octet-stream", "x-passwd": upw}); c != http.StatusNoContent {
			t.Fatalf("upload the bytes: %d %s", c, b)
		}
		fetch := func() {
			if c, b, _ := psiDo("GET", base+loc, "", nil); c != http.StatusOK || b != content {
				t.Fatalf("GET %s: %d %q", loc, c, b)
			}
		}
		fetch()
		if c, _, _ := psiDo("GET", base+"/admin/data.json", "", map[string]string{"x-passwd": "wrong-password-1"}); c != http.StatusUnauthorized && c != http.StatusForbidden {
			t.Fatalf("admin data with a wrong password answered %d", c)
		}
		if c, b, _ := psiDo("GET", base+"/admin/data.json", "", map[string]string{"x-passwd": apw}); c != http.StatusOK || !strings.Contains(b, "quenchsmoke") {
			t.Fatalf("admin data: %d %s", c, b)
		}
		docker("rm", "-f", name)
		time.Sleep(time.Second)
		start(t.Name("again"))
		fetch()
		t.Logf("smoke test passed (psitransfer %s: read-only boot, upload password enforced, tus upload downloaded back, admin data behind the admin password, file kept across a restart; user %s)", t.Want, u)
	})
}
