package smoke

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

func init() {
	// On a read-only rootfs with our postgresql and /var/lib/zipline on a volume: the first setup creates the
	// super-admin and a second one is refused, a wrong password is refused, the session uploads a file that is served
	// back byte for byte, and the account and the file survive a restart.
	register("zipline", func(t *T) {
		const base = "http://127.0.0.1:18100"
		const pw, dbpw = "Smoke-Zipline-2026!", "smoke-db-pass"
		const content = "quench zipline smoke file\n"
		u := t.User("1001")
		net := t.Name("net")
		t.Docker("network", "create", net)
		t.Cleanup(func() { docker("network", "rm", net) })
		pg := t.Name("pg")
		t.Cleanup(func() { docker("rm", "-f", pg) })
		t.Docker("run", "-d", "--name", pg, "--network", net, "-e", "POSTGRES_PASSWORD="+dbpw, "-e", "POSTGRES_DB=zipline", "ghcr.io/quenchworks/images/postgresql:18.7")
		// Zipline exits on a refused DB connection: wait for the server initdb restarts into
		t.waitCount(pg, "ready to accept connections", 2, 60)
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		start := func(name string) {
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--network", net, "-v", vol + ":/var/lib/zipline",
				"-e", "DATABASE_URL=postgresql://postgres:" + dbpw + "@" + pg + ":5432/zipline", "-e", "CORE_SECRET=quench-smoke-secret-0123456789abcdef",
				"-p", "127.0.0.1:18100:3000"})
			t.WaitHTTP(base+"/api/healthcheck", 120, name)
		}
		client := func() *http.Client {
			jar, _ := cookiejar.New(nil)
			return &http.Client{Jar: jar, Timeout: 20 * time.Second}
		}
		post := func(c *http.Client, path, body string) (int, string) {
			r, err := c.Post(base+path, "application/json", strings.NewReader(body))
			if err != nil {
				return 0, err.Error()
			}
			defer r.Body.Close()
			b, _ := io.ReadAll(r.Body)
			return r.StatusCode, string(b)
		}
		login := func(pass string) (int, *http.Client) {
			c := client()
			code, _ := post(c, "/api/auth/login", `{"username":"smoke","password":"`+pass+`"}`)
			return code, c
		}
		name := t.Name("")
		start(name)
		if c, b := post(client(), "/api/setup", `{"username":"smoke","password":"`+pw+`"}`); c != http.StatusOK {
			t.Fatalf("first setup: %d %s\n%s", c, b, t.Logs(name))
		}
		if c, _ := post(client(), "/api/setup", `{"username":"intruder","password":"`+pw+`"}`); c < 400 {
			t.Fatalf("a second setup answered %d", c)
		}
		if c, _ := login("wrong-password-1"); c < 400 {
			t.Fatalf("a wrong password answered %d", c)
		}
		code, c := login(pw)
		if code != http.StatusOK {
			t.Fatalf("login: %d", code)
		}
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		fw, _ := mw.CreateFormFile("file", "smoke.txt")
		fw.Write([]byte(content))
		mw.Close()
		req, _ := http.NewRequest("POST", base+"/api/upload", &body)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		r, err := c.Do(req)
		if err != nil {
			t.Fatalf("upload: %v", err)
		}
		ub, _ := io.ReadAll(r.Body)
		r.Body.Close()
		var up struct {
			Files []struct{ URL string }
		}
		if r.StatusCode != http.StatusOK || json.Unmarshal(ub, &up) != nil || len(up.Files) != 1 {
			t.Fatalf("upload: %d %s", r.StatusCode, ub)
		}
		pu, _ := url.Parse(up.Files[0].URL)
		fetch := func() {
			raw := base + strings.Replace(pu.Path, "/u/", "/raw/", 1)
			if c, b := Get(raw); c != http.StatusOK || b != content {
				t.Fatalf("GET %s: %d %q", raw, c, b)
			}
		}
		fetch()
		docker("rm", "-f", name)
		start(t.Name("again"))
		if code, _ := login(pw); code != http.StatusOK {
			t.Fatalf("login after a restart: %d", code)
		}
		fetch()
		t.Logf("smoke test passed (zipline %s: read-only boot on postgresql, first setup once, wrong password refused, upload served back, account and file kept across a restart; user %s)", t.Want, u)
	})
}
