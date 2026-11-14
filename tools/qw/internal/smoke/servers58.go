package smoke

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

func init() {
	// Vaultwarden runs on a read-only root with /data on a volume. The test does what a Bitwarden
	// client does: derive the master password hash (PBKDF2), register, log in with the password
	// grant, store a cipher and read it back from /api/sync, also after a restart. A wrong password
	// must be refused and the web vault must be served.
	register("vaultwarden", func(t *T) {
		u := t.User("1001")
		v := t.RunEntryAny("/usr/bin/vaultwarden", "--version")
		t.Logf("%s", v)
		t.WantVersion(v)

		vol, name := t.Name("data"), t.Name("")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		var base string
		start := func() {
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:uid=1001,gid=1001", "-v", vol + ":/data", "-p", "127.0.0.1::8080"})
			base = "http://127.0.0.1:" + t.hostPort(name, "8080")
			t.WaitHTTP(base+"/alive", 60, name)
		}
		start()
		_, page := Get(base + "/")
		t.ContainsFold(page, "<html", "web vault")
		t.Logf("  /alive ok, web vault served")

		const email = "smoke@quench.test"
		const enc = "2.c21va2VpdjEyMzQ1Njc4OQ==|c21va2VjdA==|c21va2VtYWNtYWNtYWNtYWNtYWNtYWNtYWNtYWM="
		// Bitwarden's master password hash: PBKDF2-SHA256(PBKDF2-SHA256(password, email, 600000), password, 1)
		mhash := func(pw string) string {
			k, _ := pbkdf2.Key(sha256.New, pw, []byte(email), 600000, 32)
			h, _ := pbkdf2.Key(sha256.New, string(k), []byte(pw), 1, 32)
			return base64.StdEncoding.EncodeToString(h)
		}
		js := map[string]string{"Content-Type": "application/json", "Accept": "application/json"}
		post := func(path string, v any, hdr map[string]string) (int, string) {
			b, _ := json.Marshal(v)
			return DoH("POST", base+path, string(b), hdr)
		}
		login := func(pw string) (int, string) {
			dev := make([]byte, 16)
			rand.Read(dev)
			f := url.Values{"grant_type": {"password"}, "username": {email}, "password": {mhash(pw)}, "scope": {"api offline_access"},
				"client_id": {"web"}, "deviceType": {"9"}, "deviceIdentifier": {fmt.Sprintf("%x-%x-%x-%x-%x", dev[0:4], dev[4:6], dev[6:8], dev[8:10], dev[10:])}, "deviceName": {"smoke"}}
			return DoH("POST", base+"/identity/connect/token", f.Encode(), map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Accept": "application/json"})
		}
		const pw = "smoke-master-pass-2026"
		token := func() string {
			code, body := login(pw)
			var r struct{ AccessToken string `json:"access_token"` }
			if json.Unmarshal([]byte(body), &r); code != 200 || r.AccessToken == "" {
				t.Fatalf("login failed %d %.300s", code, body)
			}
			return r.AccessToken
		}
		sync := func() {
			code, body := DoH("GET", base+"/api/sync", "", map[string]string{"Authorization": "Bearer " + token(), "Accept": "application/json"})
			var r struct{ Ciphers []json.RawMessage `json:"ciphers"` }
			if json.Unmarshal([]byte(body), &r); code != 200 || len(r.Ciphers) != 1 {
				t.Fatalf("cipher not in /api/sync (%d, %d ciphers): %.300s", code, len(r.Ciphers), body)
			}
		}

		// two steps since 1.35: with mail off the verification call returns the token itself
		code, vtok := post("/identity/accounts/register/send-verification-email", map[string]string{"email": email, "name": "Smoke"}, js)
		if code != 200 {
			t.Fatalf("verification %d %.300s", code, vtok)
		}
		var vt any
		json.Unmarshal([]byte(vtok), &vt)
		code, body := post("/identity/accounts/register/finish", map[string]any{"email": email, "name": "Smoke", "masterPasswordHash": mhash(pw),
			"masterPasswordHint": nil, "key": enc, "kdf": 0, "kdfIterations": 600000,
			"keys": map[string]string{"publicKey": "c21va2UtcHVibGljLWtleQ==", "encryptedPrivateKey": enc}, "emailVerificationToken": vt}, js)
		if code != 200 && code != 204 {
			t.Fatalf("register failed %d %.300s\n--- logs:\n%s", code, body, t.Logs(name))
		}
		if bad, _ := login("wrong-master-pass"); bad != 400 && bad != 401 {
			t.Fatalf("a wrong password got HTTP %d", bad)
		}
		tok := token()
		claims, _ := base64.RawURLEncoding.DecodeString(strings.Split(tok, ".")[1])
		var c struct{ Sub string }
		json.Unmarshal(claims, &c)
		code, body = post("/api/ciphers", map[string]any{"type": 2, "name": enc, "notes": nil, "secureNote": map[string]int{"type": 0},
			"favorite": false, "encryptedFor": c.Sub}, map[string]string{"Content-Type": "application/json", "Accept": "application/json", "Authorization": "Bearer " + tok})
		if code != 200 {
			t.Fatalf("storing a cipher failed %d %.300s\n--- logs:\n%s", code, body, t.Logs(name))
		}
		sync()
		t.Logf("  registered, wrong password refused, cipher stored and synced")
		docker("rm", "-f", name)
		start()
		sync()
		t.Logf("  cipher kept across a restart")
		if l := strings.ToLower(t.Logs(name)); strings.Contains(l, "read-only file system") || strings.Contains(l, "panicked") {
			t.Fatalf("crashed or wrote to the root:\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed (vaultwarden %s: Bitwarden register/login, cipher stored and synced across a restart on /data, read-only root; user %s)", t.Want, u)
	})
}
