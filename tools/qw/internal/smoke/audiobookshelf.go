package smoke

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// absDo sends one JSON request to Audiobookshelf, with a bearer token when one is given.
func absDo(method, url, token, body string) (int, string) {
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		return 0, err.Error()
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-return-tokens", "true")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return insecureDo(req)
}

func init() {
	// On a read-only rootfs with /config and /metadata on volumes: /init creates the root user, a wrong password is
	// refused, the root signs in, a two-second audiobook made with the image's own ffmpeg is scanned into a library
	// (ffprobe must read its duration), and after a restart the root still signs in and the item is still there.
	register("audiobookshelf", func(t *T) {
		const base = "http://127.0.0.1:18089"
		const pw = "Smoke-Root-2026!"
		u := t.User("1001")
		vols := map[string]string{}
		for _, v := range []string{"config", "metadata", "media"} {
			vols[v] = t.Name(v)
			t.Docker("volume", "create", vols[v])
			name := vols[v]
			t.Cleanup(func() { docker("volume", "rm", name) })
		}
		// the media volume is root-owned when created: hand it to 1001 with the image's ffmpeg writing the book
		t.Docker("run", "--rm", "--user", "0", "--entrypoint", "/usr/bin/ffmpeg", "-v", vols["media"]+":/media", t.Image,
			"-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=2",
			"-metadata", "title=Smoke Book", "-metadata", "artist=Smoke Author", "/media/book.mp3")
		t.Docker("run", "--rm", "--user", "0", "--entrypoint", "/usr/bin/node", "-v", vols["media"]+":/media", t.Image,
			"-e", `const fs=require("fs");fs.mkdirSync("/media/Smoke Author/Smoke Book",{recursive:true});fs.renameSync("/media/book.mp3","/media/Smoke Author/Smoke Book/book.mp3");for(const p of ["/media","/media/Smoke Author","/media/Smoke Author/Smoke Book","/media/Smoke Author/Smoke Book/book.mp3"])fs.chownSync(p,1001,1001)`)
		flags := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-p", "127.0.0.1:18089:13378",
			"-v", vols["config"] + ":/config", "-v", vols["metadata"] + ":/metadata", "-v", vols["media"] + ":/media:ro"}
		start := func(name string) {
			t.Start(name, flags)
			for i := 0; i < 90; i++ {
				if c, _ := absDo("GET", base+"/healthcheck", "", ""); c == http.StatusOK {
					return
				}
				time.Sleep(2 * time.Second)
			}
			t.Fatalf("audiobookshelf did not answer /healthcheck:\n%s", t.Logs(name))
		}
		login := func(pass string) (int, string) {
			c, b := absDo("POST", base+"/login", "", `{"username":"root","password":"`+pass+`"}`)
			var r struct {
				User struct {
					AccessToken string `json:"accessToken"`
				} `json:"user"`
			}
			_ = json.Unmarshal([]byte(b), &r)
			return c, r.User.AccessToken
		}
		name := t.Name("")
		start(name)
		_, st := absDo("GET", base+"/status", "", "")
		t.Contains(st, `"serverVersion":"`+t.Want+`"`, "/status")
		t.Contains(st, `"isInit":false`, "/status before init")
		if c, b := absDo("POST", base+"/init", "", `{"newRoot":{"username":"root","password":"`+pw+`"}}`); c != http.StatusOK {
			t.Fatalf("/init: %d %s", c, b)
		}
		if c, _ := login("wrong-password-1"); c != http.StatusUnauthorized {
			t.Fatalf("a wrong password got %d, want 401", c)
		}
		c, tok := login(pw)
		if c != http.StatusOK || tok == "" {
			t.Fatalf("root sign-in: %d, token %q\n%s", c, tok, t.Logs(name))
		}
		c, b := absDo("POST", base+"/api/libraries", tok, `{"name":"Smoke","mediaType":"book","folders":[{"fullPath":"/media"}]}`)
		var lib struct {
			ID string `json:"id"`
		}
		if c != http.StatusOK || json.Unmarshal([]byte(b), &lib) != nil || lib.ID == "" {
			t.Fatalf("create library: %d %s", c, b)
		}
		items := func(tok string) (int, float64) {
			_, b := absDo("GET", base+"/api/libraries/"+lib.ID+"/items", tok, "")
			var r struct {
				Total   int `json:"total"`
				Results []struct {
					Media struct {
						Duration float64 `json:"duration"`
					} `json:"media"`
				} `json:"results"`
			}
			_ = json.Unmarshal([]byte(b), &r)
			if len(r.Results) == 0 {
				return r.Total, 0
			}
			return r.Total, r.Results[0].Media.Duration
		}
		// adding a library starts a scan; ask again in case it had not seen the folder yet
		absDo("POST", base+"/api/libraries/"+lib.ID+"/scan", tok, "")
		var n int
		var dur float64
		for i := 0; i < 45 && (n == 0 || dur == 0); i++ {
			time.Sleep(2 * time.Second)
			n, dur = items(tok)
		}
		if n != 1 || dur < 1.5 {
			t.Fatalf("library scan: %d items, duration %.2f (want 1 item of ~2s)\n%s", n, dur, t.Logs(name))
		}
		docker("rm", "-f", name)
		start(t.Name("again"))
		c, tok = login(pw)
		if c != http.StatusOK || tok == "" {
			t.Fatalf("root sign-in after a restart: %d", c)
		}
		if n, _ := items(tok); n != 1 {
			t.Fatalf("library item lost across a restart: %d items", n)
		}
		t.Logf("smoke test passed (audiobookshelf %s: read-only boot, root created by /init, wrong password refused, a 2s book made by the image's ffmpeg scanned with duration %.1fs, root and item kept across a restart; user %s)", t.Want, dur, u)
	})
}
