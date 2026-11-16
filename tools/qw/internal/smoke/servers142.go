package smoke

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// komgaDo sends a request with optional basic auth and extra headers and returns the status and body.
func komgaDo(method, url, user, pass, body string, hdr map[string]string) (int, string) {
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	if user != "" {
		req.SetBasicAuth(user, pass)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	r, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return 0, err.Error()
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return r.StatusCode, string(b)
}

// writeCBZ writes a one-page comic (a PNG inside a zip) to path.
func writeCBZ(path string) error {
	img := image.NewRGBA(image.Rect(0, 0, 120, 180))
	for y := 0; y < 180; y++ {
		for x := 0; x < 120; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 2), uint8(y), 160, 255})
		}
	}
	var page bytes.Buffer
	if err := png.Encode(&page, img); err != nil {
		return err
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("001.png")
	if err != nil {
		return err
	}
	w.Write(page.Bytes())
	if err := zw.Close(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func init() {
	// On a read-only rootfs with /var/lib/komga on a volume and a read-only books mount: the first account is
	// claimed (a second claim is refused), a wrong password is refused, a library on the mount is scanned into
	// a book whose page thumbnail is generated, and the account and the book survive a restart.
	register("komga", func(t *T) {
		const base = "http://127.0.0.1:18094"
		const user, pw = "admin@quench-works.com", "Smoke-Komga-2026!"
		u := t.User("1001")
		books := t.TempDir()
		_ = os.Chmod(books, 0o755)
		if err := writeCBZ(filepath.Join(books, "Smoke Series", "Smoke 01.cbz")); err != nil {
			t.Fatalf("comic: %v", err)
		}
		_ = os.Chmod(filepath.Join(books, "Smoke Series"), 0o755)
		net := t.Name("net")
		t.Docker("network", "create", net)
		t.Cleanup(func() { docker("network", "rm", net) })
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		start := func(name string) {
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,exec,mode=1777", "--network", net, "-v", vol + ":/var/lib/komga",
				"-v", books + ":/books:ro", "-p", "127.0.0.1:18094:25600"})
			t.waitFor(180, time.Second, name, "komga never answered /api/v1/claim", func() bool {
				c, _ := komgaDo("GET", base+"/api/v1/claim", "", "", "", nil)
				return c == http.StatusOK
			})
		}
		listBooks := func(lib string) (int, string) {
			c, b := komgaDo("GET", base+"/api/v1/books?library_id="+lib, user, pw, "", nil)
			var page struct {
				TotalElements int
				Content       []struct{ ID string }
			}
			if c != http.StatusOK || json.Unmarshal([]byte(b), &page) != nil || len(page.Content) == 0 {
				return 0, ""
			}
			return page.TotalElements, page.Content[0].ID
		}
		name := t.Name("")
		start(name)
		// the image's claim helper (the chart runs it as a hook Job) claims the server, and is a no-op after that
		helper := func() (string, error) {
			return t.RunFlags([]string{"--network", net, "--entrypoint", "/usr/lib/jvm/java-21-openjdk/bin/java",
				"-e", "KOMGA_CLAIM_URL=http://" + name + ":25600", "-e", "KOMGA_CLAIM_EMAIL=" + user, "-e", "KOMGA_CLAIM_PASSWORD=" + pw},
				"-cp", "/usr/lib/komga-claim", "KomgaClaim")
		}
		if out, err := helper(); err != nil || !strings.Contains(out, "claimed by "+user) {
			t.Fatalf("claim helper: %v %s\n%s", err, out, t.Logs(name))
		}
		if out, err := helper(); err != nil || !strings.Contains(out, "already claimed") {
			t.Fatalf("claim helper on a claimed server: %v %s", err, out)
		}
		if c, _ := komgaDo("POST", base+"/api/v1/claim", "", "", "", map[string]string{"X-Komga-Email": "x@example.com", "X-Komga-Password": "Other-2026!"}); c != http.StatusBadRequest {
			t.Fatalf("a second claim got %d, want 400", c)
		}
		if c, _ := komgaDo("GET", base+"/api/v2/users/me", user, "wrong-password-1", "", nil); c != http.StatusUnauthorized {
			t.Fatalf("a wrong password got %d, want 401", c)
		}
		c, b := komgaDo("POST", base+"/api/v1/libraries", user, pw, `{"name":"Smoke","root":"/books"}`, nil)
		var lib struct{ ID string }
		if c != http.StatusOK || json.Unmarshal([]byte(b), &lib) != nil || lib.ID == "" {
			t.Fatalf("create library: %d %s", c, b)
		}
		var book string
		t.waitFor(120, time.Second, name, "the library scan found no book", func() bool {
			n, id := listBooks(lib.ID)
			book = id
			return n == 1
		})
		var thumb string
		t.waitFor(60, time.Second, name, "no thumbnail for the book", func() bool {
			c, b := komgaDo("GET", base+"/api/v1/books/"+book+"/thumbnail", user, pw, "", nil)
			thumb = b
			return c == http.StatusOK && len(b) > 100
		})
		if !strings.HasPrefix(thumb, "\xff\xd8") {
			t.Fatalf("the thumbnail is not a JPEG")
		}
		docker("rm", "-f", name)
		start(t.Name("again"))
		if n, _ := listBooks(lib.ID); n != 1 {
			t.Fatalf("the book is gone after a restart (%d)", n)
		}
		t.Logf("smoke test passed (komga %s: read-only boot, server claimed once by the claim helper (a second run and a second claim are no-ops/refused), wrong password refused, library scanned into a book with a JPEG thumbnail, account and book kept across a restart; user %s)", t.Want, u)
	})
}
