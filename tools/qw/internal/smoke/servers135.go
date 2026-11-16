package smoke

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// koelLogin posts the credentials to /api/me and returns the status and the token.
func koelLogin(base, email, pass string) (int, string) {
	b, _ := json.Marshal(map[string]string{"email": email, "password": pass})
	req, _ := http.NewRequest("POST", base+"/api/me", strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, ""
	}
	defer r.Body.Close()
	var out struct {
		Token string `json:"token"`
	}
	raw, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(raw, &out)
	return r.StatusCode, out.Token
}

func init() {
	// On a read-only rootfs with storage on a volume (SQLite): migrations run, the first admin comes from
	// ADMIN_EMAIL/ADMIN_PASSWORD, the API refuses a wrong password and returns a token that opens /api/me,
	// and the account survives a restart.
	register("koel", func(t *T) {
		const base = "http://127.0.0.1:18087"
		const email = "smoke@quench-works.com"
		const pw = "Smoke-Koel-2026"
		u := t.User("1001")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		flags := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/var/lib/koel", "-p", "127.0.0.1:18087:8080",
			"-e", "APP_KEY=base64:UXVlbmNoS29lbFNtb2tlS2V5MDEyMzQ1Njc4OWFiYzA=", "-e", "APP_URL=http://127.0.0.1:18087",
			"-e", "ADMIN_EMAIL=" + email, "-e", "ADMIN_PASSWORD=" + pw}
		start := func(name string) {
			t.Start(name, flags)
			for i := 0; i < 120; i++ {
				if c, _ := restDo("GET", base+"/", "", "", ""); c == http.StatusOK {
					return
				}
				time.Sleep(time.Second)
			}
			t.Fatalf("koel did not serve /:\n%s", t.Logs(name))
		}
		me := func(tok string) (int, string) {
			req, _ := http.NewRequest("GET", base+"/api/me", nil)
			req.Header.Set("Authorization", "Bearer "+tok)
			req.Header.Set("Accept", "application/json")
			return insecureDo(req)
		}
		name := t.Name("")
		start(name)
		if c, _ := koelLogin(base, email, "wrong-password-1"); c < 400 {
			t.Fatalf("a wrong password got %d", c)
		}
		c, tok := koelLogin(base, email, pw)
		if c != http.StatusOK || tok == "" {
			t.Fatalf("login: %d (no token)\n%s", c, t.Logs(name))
		}
		if c, body := me(tok); c != http.StatusOK || !strings.Contains(body, email) {
			t.Fatalf("/api/me with the token: %d %q", c, body)
		}
		docker("rm", "-f", name)
		start(t.Name("again"))
		if c, tok := koelLogin(base, email, pw); c != http.StatusOK || tok == "" {
			t.Fatalf("login after a restart: %d", c)
		}
		t.Logf("smoke test passed (koel %s: read-only boot on SQLite, first admin from the environment, wrong password refused, token opens /api/me, account kept across a restart; user %s)", t.Want, u)
	})
}
