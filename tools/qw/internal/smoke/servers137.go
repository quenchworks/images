package smoke

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// shaarliJWT signs the token Shaarli's API expects: HS512 over {"iat": now} with the API secret.
func shaarliJWT(secret string) string {
	enc := base64.RawURLEncoding
	h := enc.EncodeToString([]byte(`{"typ":"JWT","alg":"HS512"}`))
	p := enc.EncodeToString([]byte(fmt.Sprintf(`{"iat":%d}`, time.Now().Unix())))
	m := hmac.New(sha512.New, []byte(secret))
	m.Write([]byte(h + "." + p))
	return h + "." + p + "." + enc.EncodeToString(m.Sum(nil))
}

func shaarliAPI(method, url, secret, body string) (int, string) {
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+shaarliJWT(secret))
	req.Header.Set("Content-Type", "application/json")
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err.Error()
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return r.StatusCode, string(b)
}

func init() {
	// On a read-only rootfs with /var/lib/shaarli on a volume: the config comes from the environment (no
	// install form), the API refuses a token signed with the wrong secret, a bookmark created through the
	// API is listed, the data directory is not served, and the bookmark survives a restart.
	register("shaarli", func(t *T) {
		const base = "http://127.0.0.1:18089"
		const secret = "quench-smoke-api-secret-2026"
		const link = "https://quench-works.com/smoke-shaarli"
		u := t.User("1001")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		flags := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/var/lib/shaarli", "-p", "127.0.0.1:18089:8080",
			"-e", "SHAARLI_USER=smoke", "-e", "SHAARLI_PASSWORD=Smoke-Shaarli-2026", "-e", "SHAARLI_API_SECRET=" + secret}
		start := func(name string) {
			t.Start(name, flags)
			for i := 0; i < 90; i++ {
				if c, _ := restDo("GET", base+"/", "", "", ""); c == http.StatusOK {
					return
				}
				time.Sleep(time.Second)
			}
			t.Fatalf("shaarli did not serve /:\n%s", t.Logs(name))
		}
		name := t.Name("")
		start(name)
		if c, page := restDo("GET", base+"/", "", "", ""); c != http.StatusOK || strings.Contains(page, "install") && strings.Contains(page, "setlogin") {
			t.Fatalf("the front page is the install form (config not written): %d", c)
		}
		if c, _ := shaarliAPI("GET", base+"/api/v1/info", "wrong-secret-1", ""); c != http.StatusUnauthorized {
			t.Fatalf("a token with the wrong secret got %d, want 401", c)
		}
		if c, body := shaarliAPI("POST", base+"/api/v1/links", secret, `{"url":"`+link+`","title":"Quench smoke"}`); c != http.StatusCreated {
			t.Fatalf("create a bookmark: %d %s\n%s", c, body, t.Logs(name))
		}
		if c, body := shaarliAPI("GET", base+"/api/v1/links", secret, ""); c != http.StatusOK || !strings.Contains(body, "smoke-shaarli") {
			t.Fatalf("list bookmarks: %d %s", c, body)
		}
		if c, _ := restDo("GET", base+"/data/config.json.php", "", "", ""); c != http.StatusForbidden {
			t.Fatalf("/data/config.json.php answered %d, want 403", c)
		}
		docker("rm", "-f", name)
		start(t.Name("again"))
		if c, body := shaarliAPI("GET", base+"/api/v1/links", secret, ""); c != http.StatusOK || !strings.Contains(body, "smoke-shaarli") {
			t.Fatalf("list after a restart: %d %s", c, body)
		}
		t.Logf("smoke test passed (shaarli %s: read-only boot, config from the environment, wrong API secret refused, bookmark created and listed through the API, data/ not served, kept across a restart; user %s)", t.Want, u)
	})
}
