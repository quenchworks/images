package smoke

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
)

// microbinPost sends a multipart form without following redirects and returns the status and Location.
func microbinPost(url string, fields [][2]string) (int, string) {
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	for _, f := range fields {
		_ = w.WriteField(f[0], f[1])
	}
	_ = w.Close()
	r, err := noRedirect.Post(url, w.FormDataContentType(), &b)
	if err != nil {
		return 0, err.Error()
	}
	defer r.Body.Close()
	_, _ = io.Copy(io.Discard, r.Body)
	return r.StatusCode, r.Header.Get("Location")
}

func init() {
	// On a read-only rootfs with the data dir on a volume: a paste is uploaded and read back raw, the admin
	// page refuses a wrong password and opens with the right one, and the paste survives a restart.
	register("microbin", func(t *T) {
		const base = "http://127.0.0.1:18080"
		const admin = "Smoke-Admin-2026"
		const text = "quench microbin smoke paste"
		u := t.User("1001")
		t.Contains(t.RunAny("--version"), t.Want, "microbin --version")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		flags := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/var/lib/microbin",
			"-e", "MICROBIN_ADMIN_PASSWORD=" + admin, "-p", "127.0.0.1:18080:8080"}
		start := func(name string) {
			t.Start(name, flags)
			t.WaitHTTP(base+"/", 60, name)
		}
		raw := func(slug string) string {
			r, err := http.Get(base + "/raw/" + slug)
			if err != nil {
				return err.Error()
			}
			defer r.Body.Close()
			b, _ := io.ReadAll(r.Body)
			return string(b)
		}
		name := t.Name("")
		start(name)
		code, loc := microbinPost(base+"/upload", [][2]string{{"expiration", "24hour"}, {"burn_after", "0"}, {"syntax_highlight", "none"}, {"privacy", "public"}, {"content", text}})
		slug := loc[strings.LastIndex(loc, "/")+1:]
		if code != http.StatusFound || !strings.Contains(loc, "/upload/") || slug == "" {
			t.Fatalf("upload: %d %q", code, loc)
		}
		if got := raw(slug); !strings.Contains(got, text) {
			t.Fatalf("raw paste %s: %q", slug, got)
		}
		if code, loc := microbinPost(base+"/admin", [][2]string{{"username", "admin"}, {"password", "wrong-password-1"}}); code != http.StatusFound || !strings.Contains(loc, "incorrect") {
			t.Fatalf("a wrong admin password got %d %q, want a redirect to incorrect", code, loc)
		}
		if code, loc := microbinPost(base+"/admin", [][2]string{{"username", "admin"}, {"password", admin}}); code != http.StatusOK {
			t.Fatalf("admin login got %d %q, want 200", code, loc)
		}
		docker("rm", "-f", name)
		start(t.Name("again"))
		if got := raw(slug); !strings.Contains(got, text) {
			t.Fatalf("raw paste %s after a restart: %q", slug, got)
		}
		t.Logf("smoke test passed (microbin %s: read-only boot, paste uploaded and read back raw, wrong admin password refused, admin page opens, paste kept across a restart; user %s)", t.Want, u)
	})
}
