package smoke

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func init() {
	// On a read-only rootfs with /var/lib/trilium on a volume: a new install creates the document (without the demo
	// notes) and sets the password, ETAPI logs in and creates a note that it reads back, the note and the password
	// survive a restart, and a wrong password is refused (last: logins are rate-limited).
	register("trilium", func(t *T) {
		const base = "http://127.0.0.1:18109"
		const pw = "Smoke-Trilium-2026!"
		const body = "<p>quench trilium smoke note</p>"
		u := t.User("1001")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		start := func(name string) {
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/var/lib/trilium", "-p", "127.0.0.1:18109:8080"})
			// "/" serves the setup screen until the document exists (GET /setup renders a view the build no longer ships)
			t.WaitHTTP(base+"/", 90, name)
		}
		json1 := map[string]string{"Content-Type": "application/json"}
		form := map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
		etapiLogin := func(pass string) (int, string) {
			c, b := DoH("POST", base+"/etapi/auth/login", `{"password":"`+pass+`"}`, json1)
			var r struct{ AuthToken string }
			json.Unmarshal([]byte(b), &r)
			return c, r.AuthToken
		}
		name := t.Name("")
		start(name)
		if c, b := DoH("POST", base+"/api/setup/new-document?skipDemoDb", `{}`, json1); c/100 != 2 {
			t.Fatalf("create the document: %d %s\n%s", c, b, t.Logs(name))
		}
		v := url.Values{"password1": {pw}, "password2": {pw}}.Encode()
		if c, b := DoH("POST", base+"/set-password", v, form); c >= 400 {
			t.Fatalf("set the password: %d %s", c, b)
		}
		c, tok := etapiLogin(pw)
		if c != http.StatusCreated || tok == "" {
			t.Fatalf("ETAPI login: %d\n%s", c, t.Logs(name))
		}
		auth := map[string]string{"Authorization": tok, "Content-Type": "application/json"}
		c, b := DoH("POST", base+"/etapi/create-note", `{"parentNoteId":"root","title":"Quench smoke","type":"text","content":"`+body+`"}`, auth)
		var created struct{ Note struct{ NoteID string } }
		if c != http.StatusCreated || json.Unmarshal([]byte(b), &created) != nil || created.Note.NoteID == "" {
			t.Fatalf("create a note: %d %s", c, b)
		}
		read := func(tok string) {
			c, b := DoH("GET", base+"/etapi/notes/"+created.Note.NoteID+"/content", "", map[string]string{"Authorization": tok})
			if c != http.StatusOK || !strings.Contains(b, "quench trilium smoke note") {
				t.Fatalf("read the note: %d %q", c, b)
			}
		}
		read(tok)
		docker("rm", "-f", name)
		time.Sleep(time.Second)
		start(t.Name("again"))
		c, tok = etapiLogin(pw)
		if c != http.StatusCreated || tok == "" {
			t.Fatalf("ETAPI login after a restart: %d", c)
		}
		read(tok)
		if c, tok := etapiLogin("wrong-password-1"); c != http.StatusUnauthorized || tok != "" {
			t.Fatalf("a wrong password answered %d", c)
		}
		t.Logf("smoke test passed (trilium %s: read-only boot, document created and password set, ETAPI note created and read back, kept across a restart, wrong password refused; user %s)", t.Want, u)
	})
}
