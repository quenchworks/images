package smoke

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

func init() {
	// On a read-only rootfs with /app/data on a volume: the first visitor registers and a second registration is
	// refused, a wrong password is refused, the session converts a PNG to JPEG through ImageMagick and a text file to
	// PDF through headless LibreOffice (both downloaded and checked by their magic bytes), and the account survives a
	// restart.
	register("convertx", func(t *T) {
		const base = "http://127.0.0.1:18107"
		const pw = "Smoke-ConvertX-2026!"
		u := t.User("1001")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		start := func(name string) {
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/app/data", "-p", "127.0.0.1:18107:3000",
				"-e", "JWT_SECRET=quench-smoke-jwt-secret-0123456789abcdef", "-e", "HTTP_ALLOWED=true"})
			t.WaitHTTP(base+"/healthcheck", 60, name)
		}
		client := func() *http.Client {
			jar, _ := cookiejar.New(nil)
			return &http.Client{Jar: jar, Timeout: 120 * time.Second,
				CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		}
		form := func(c *http.Client, path string, v url.Values) (int, string) {
			r, err := c.PostForm(base+path, v)
			if err != nil {
				return 0, err.Error()
			}
			defer r.Body.Close()
			return r.StatusCode, r.Header.Get("Location")
		}
		get := func(c *http.Client, path string) (int, []byte) {
			r, err := c.Get(base + path)
			if err != nil {
				return 0, nil
			}
			defer r.Body.Close()
			b, _ := io.ReadAll(r.Body)
			return r.StatusCode, b
		}
		// a session is a redirect home with the auth cookie set
		signedIn := func(c *http.Client) bool {
			bu, _ := url.Parse(base)
			for _, k := range c.Jar.Cookies(bu) {
				if k.Name == "auth" && k.Value != "" {
					return true
				}
			}
			return false
		}
		upload := func(c *http.Client, name string, data []byte) {
			var body bytes.Buffer
			mw := multipart.NewWriter(&body)
			fw, _ := mw.CreateFormFile("file", name)
			fw.Write(data)
			mw.Close()
			req, _ := http.NewRequest("POST", base+"/upload", &body)
			req.Header.Set("Content-Type", mw.FormDataContentType())
			r, err := c.Do(req)
			if err != nil {
				t.Fatalf("upload %s: %v", name, err)
			}
			r.Body.Close()
			if r.StatusCode/100 != 2 {
				t.Fatalf("upload %s: %d", name, r.StatusCode)
			}
		}
		jobID := func(c *http.Client) string {
			bu, _ := url.Parse(base)
			for _, k := range c.Jar.Cookies(bu) {
				if k.Name == "jobId" {
					return k.Value
				}
			}
			return ""
		}
		// convert one uploaded file in a fresh job and wait for the output, checking its first bytes
		convert := func(c *http.Client, name string, data []byte, to, converter, out, magic string) {
			get(c, "/") // a fresh job
			job := jobID(c)
			if job == "" {
				t.Fatalf("GET / set no jobId cookie")
			}
			upload(c, name, data)
			if code, loc := form(c, "/convert", url.Values{"convert_to": {to + "," + converter}, "file_names": {`["` + name + `"]`}}); code/100 != 3 || !strings.Contains(loc, "/results/") {
				t.Fatalf("convert %s to %s: %d %s", name, to, code, loc)
			}
			for i := 0; i < 90; i++ {
				if code, b := get(c, "/download/1/"+job+"/"+out); code == http.StatusOK && bytes.HasPrefix(b, []byte(magic)) {
					return
				}
				time.Sleep(time.Second)
			}
			t.Fatalf("%s never converted to %s with %s", name, to, converter)
		}
		name := t.Name("")
		start(name)
		c := client()
		if code, _ := form(c, "/register", url.Values{"email": {"smoke@quench-works.com"}, "password": {pw}}); code/100 != 3 || !signedIn(c) {
			t.Fatalf("first registration: %d\n%s", code, t.Logs(name))
		}
		if x := client(); func() bool {
			form(x, "/register", url.Values{"email": {"intruder@quench-works.com"}, "password": {pw}})
			return signedIn(x)
		}() {
			t.Fatalf("a second registration got a session")
		}
		if x := client(); func() bool {
			form(x, "/login", url.Values{"email": {"smoke@quench-works.com"}, "password": {"wrong-password-1"}})
			return signedIn(x)
		}() {
			t.Fatalf("a wrong password got a session")
		}
		img := image.NewRGBA(image.Rect(0, 0, 16, 16))
		for i := range 16 {
			img.Set(i, i, color.RGBA{200, 30, 30, 255})
		}
		var pngb bytes.Buffer
		png.Encode(&pngb, img)
		convert(c, "smoke.png", pngb.Bytes(), "jpg", "imagemagick", "smoke.jpg", "\xff\xd8\xff")
		convert(c, "smoke.txt", []byte("quench convertx smoke\n"), "pdf", "libreoffice", "smoke.pdf", "%PDF")
		docker("rm", "-f", name)
		start(t.Name("again"))
		c2 := client()
		if form(c2, "/login", url.Values{"email": {"smoke@quench-works.com"}, "password": {pw}}); !signedIn(c2) {
			t.Fatalf("login after a restart failed")
		}
		t.Logf("smoke test passed (convertx %s: read-only boot, first registration only, wrong password refused, PNG to JPEG with ImageMagick and text to PDF with LibreOffice, account kept across a restart; user %s)", t.Want, u)
	})
}
