package smoke

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// miniserveDo sends a request with basic auth and returns the status and body.
func miniserveDo(method, url, user, pass, ctype string, body io.Reader) (int, string) {
	req, _ := http.NewRequest(method, url, body)
	req.SetBasicAuth(user, pass)
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	r, err := noRedirect.Do(req)
	if err != nil {
		return 0, err.Error()
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return r.StatusCode, string(b)
}

func init() {
	// On a read-only rootfs with /srv on a volume: basic auth refuses a wrong password, a file uploads and
	// downloads, the directory comes back as a zip archive, and the file survives a restart.
	register("miniserve", func(t *T) {
		const base = "http://127.0.0.1:18081"
		const pw = "Smoke-Pass-2026"
		const text = "quench miniserve smoke file"
		u := t.User("1001")
		t.Contains(t.RunAny("--version"), t.Want, "miniserve --version")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		flags := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/srv", "-p", "127.0.0.1:18081:8080",
			"-e", "MINISERVE_AUTH=smoke:" + pw, "-e", "MINISERVE_ALLOWED_UPLOAD_DIR=/", "-e", "MINISERVE_ENABLE_ZIP=true"}
		start := func(name string) {
			t.Start(name, flags)
			for i := 0; i < 60; i++ {
				if c, _ := miniserveDo("GET", base+"/", "smoke", pw, "", nil); c == http.StatusOK {
					return
				}
				time.Sleep(time.Second)
			}
			t.Fatalf("miniserve did not answer:\n%s", t.Logs(name))
		}
		name := t.Name("")
		start(name)
		if c, _ := miniserveDo("GET", base+"/", "smoke", "wrong-password-1", "", nil); c != http.StatusUnauthorized {
			t.Fatalf("a wrong password got %d, want 401", c)
		}
		var b bytes.Buffer
		w := multipart.NewWriter(&b)
		fw, _ := w.CreateFormFile("file_to_upload", "hello.txt")
		_, _ = fw.Write([]byte(text))
		_ = w.Close()
		if c, body := miniserveDo("POST", base+"/upload?path=/", "smoke", pw, w.FormDataContentType(), &b); c >= 400 {
			t.Fatalf("upload: %d %s", c, body)
		}
		if c, body := miniserveDo("GET", base+"/hello.txt", "smoke", pw, "", nil); c != http.StatusOK || body != text {
			t.Fatalf("download: %d %q", c, body)
		}
		if c, body := miniserveDo("GET", base+"/?download=zip", "smoke", pw, "", nil); c != http.StatusOK || !strings.HasPrefix(body, "PK") || !strings.Contains(body, "hello.txt") {
			t.Fatalf("zip archive: %d (%d bytes)", c, len(body))
		}
		docker("rm", "-f", name)
		start(t.Name("again"))
		if c, body := miniserveDo("GET", base+"/hello.txt", "smoke", pw, "", nil); c != http.StatusOK || body != text {
			t.Fatalf("download after a restart: %d %q", c, body)
		}
		t.Logf("smoke test passed (miniserve %s: read-only boot, wrong password refused, file uploaded and downloaded, zip archive of the directory, file kept across a restart; user %s)", t.Want, u)
	})
}
