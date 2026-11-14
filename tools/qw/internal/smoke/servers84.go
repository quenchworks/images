package smoke

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

// paletteOverflowBMP is a 4x1 8bpp BMP whose 2-colour palette is shorter than its pixel index
// (70). Before x/image v0.41.0 it decoded to an image.Paletted that panics imaging 1.6.2's
// scanner (CVE-2023-36308) and Go's own Paletted.At.
func paletteOverflowBMP() []byte {
	b := make([]byte, 14+40+2*4+4)
	le := binary.LittleEndian
	copy(b, "BM")
	le.PutUint32(b[2:], uint32(len(b)))
	le.PutUint32(b[10:], 14+40+8)
	le.PutUint32(b[14:], 40)
	le.PutUint32(b[18:], 4)
	le.PutUint32(b[22:], 1)
	le.PutUint16(b[26:], 1)
	le.PutUint16(b[28:], 8)
	le.PutUint32(b[46:], 2)
	copy(b[62:], []byte{70, 70, 70, 70})
	return b
}

func init() {
	// The default account logs in, and a bookmark is added for a page whose og:image is the
	// palette-overflow BMP served as image/png (Shiori checks the content type, then decodes by
	// magic bytes and thumbnails with imaging). The bookmark keeps the page title, the server
	// gets no thumbnail while a valid PNG does, and the server is still serving afterwards.
	register("shiori", func(t *T) {
		const base = "http://127.0.0.1:18095"
		u := t.User("1001")
		t.Contains(t.RunAny("version"), t.Want, "shiori version")
		var good bytes.Buffer
		png.Encode(&good, image.NewNRGBA(image.Rect(0, 0, 40, 30)))
		imgs := map[string][]byte{"/evil.png": paletteOverflowBMP(), "/good.png": good.Bytes()}
		var mu sync.Mutex
		served := map[string]bool{}
		var stub string // og:image must be absolute: Shiori does not resolve relative image URLs
		stub = t.hostStub(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if b, ok := imgs[r.URL.Path]; ok {
				mu.Lock()
				served[r.URL.Path] = true
				mu.Unlock()
				w.Header().Set("Content-Type", "image/png")
				w.Write(b)
				return
			}
			// /page/<name> links /<name>.png
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(`<!doctype html><html><head><title>Quench smoke ` + path.Base(r.URL.Path) + `</title>
<meta property="og:image" content="` + stub + `/` + path.Base(r.URL.Path) + `.png"></head><body><article><h1>Quench smoke page</h1>
<p>` + strings.Repeat("A paragraph long enough for the readability extractor to keep it. ", 20) + `</p></article></body></html>`))
		}))
		name := t.Name("")
		t.Start(name, append(append([]string{}, hostStubFlag...), "--read-only", "--tmpfs", "/tmp:rw,mode=1777",
			"--tmpfs", "/shiori:rw,uid=1001,gid=1001", "-p", "127.0.0.1:18095:8080"))
		t.waitFor(60, time.Second, name, "shiori never served /", func() bool { c, _ := Get(base + "/"); return c == 200 })
		_, login := DoH("POST", base+"/api/v1/auth/login", `{"username":"shiori","password":"gopher"}`, map[string]string{"Content-Type": "application/json"})
		tok := t.Match(login, `"token":"([^"]+)"`, "login with the default account")
		auth := map[string]string{"Authorization": "Bearer " + tok, "Content-Type": "application/json"}
		if c, me := DoH("GET", base+"/api/v1/auth/me", "", auth); c != 200 || !strings.Contains(me, `"shiori"`) {
			t.Fatalf("/api/v1/auth/me: %d %s", c, me)
		}
		// async runs the download in a bare goroutine: there a panic (imaging's scanner or Go's
		// Paletted.At) kills the process, while the sync path's handler goroutine would recover it
		add := func(page string, async bool) {
			c, b := DoH("POST", base+"/api/bookmarks", `{"url":"`+stub+`/page/`+page+`","createArchive":false,"async":`+strconv.FormatBool(async)+`}`, auth)
			if c != 200 {
				t.Fatalf("add bookmark %s: %d %s\n%s", page, c, b, t.Logs(name))
			}
			if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
				t.Fatalf("shiori died processing the %s thumbnail\n%s", page, t.Logs(name))
			}
		}
		add("good", false)
		add("evil", true)
		t.waitFor(20, time.Second, name, "shiori never fetched the crafted thumbnail", func() bool {
			mu.Lock()
			defer mu.Unlock()
			return served["/evil.png"]
		})
		time.Sleep(3 * time.Second)
		if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
			t.Fatalf("shiori died processing the crafted thumbnail\n%s", t.Logs(name))
		}
		if c, _ := Get(base + "/"); c != 200 {
			t.Fatalf("shiori stopped serving after the crafted thumbnail\n%s", t.Logs(name))
		}
		// the insert reply is the record before the download: read the processed ones back
		_, list := DoH("GET", base+"/api/bookmarks", "", auth)
		var got struct {
			Message struct {
				Bookmarks []struct {
					URL, Title string
					Image      string `json:"imageURL"`
				} `json:"bookmarks"`
			} `json:"message"`
		}
		if json.Unmarshal([]byte(list), &got) != nil || len(got.Message.Bookmarks) != 2 {
			t.Fatalf("bookmark list: %.600s", list)
		}
		for _, b := range got.Message.Bookmarks {
			evil := strings.HasSuffix(b.URL, "/evil")
			// the control: a valid small PNG takes imaging's resize path and becomes a thumbnail
			if evil != (b.Image == "") || !strings.HasPrefix(b.Title, "Quench smoke ") {
				t.Fatalf("bookmark %s: title %q, thumbnail %q (want one for good, none for evil)", b.URL, b.Title, b.Image)
			}
		}

		// the build reads the first owner from SHIORI_INITIAL_USERNAME/PASSWORD (upstream hardcodes
		// shiori/gopher); with them set the defaults must not log in
		own := t.Name("owner")
		t.Start(own, []string{"--tmpfs", "/shiori:rw,uid=1001,gid=1001", "-p", "127.0.0.1:18096:8080",
			"-e", "SHIORI_INITIAL_USERNAME=quench", "-e", "SHIORI_INITIAL_PASSWORD=Smoke-Owner-2026"})
		t.waitFor(60, time.Second, own, "shiori (owner from env) never served /", func() bool { c, _ := Get("http://127.0.0.1:18096/"); return c == 200 })
		loginAs := func(user, pw string) int {
			c, _ := DoH("POST", "http://127.0.0.1:18096/api/v1/auth/login", `{"username":"`+user+`","password":"`+pw+`"}`, map[string]string{"Content-Type": "application/json"})
			return c
		}
		if c := loginAs("quench", "Smoke-Owner-2026"); c != 200 {
			t.Fatalf("login as the owner from SHIORI_INITIAL_*: %d\n%s", c, t.Logs(own))
		}
		if c := loginAs("shiori", "gopher"); c == 200 {
			t.Fatalf("shiori/gopher still logs in with SHIORI_INITIAL_* set")
		}
		t.Logf("smoke test passed (shiori %s: owner from env, login, bookmark with page title, crafted BMP thumbnail rejected and the server kept serving; user %s)", t.Want, u)
	})
}
