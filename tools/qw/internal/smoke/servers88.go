package smoke

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

func init() {
	// The hub on a read-only rootfs: /api/health, a superuser created with the binary's own CLI
	// signs in, and the palette-overflow BMP uploaded as a thumbnailed file (PocketBase thumbs go
	// through imaging) leaves the hub serving. The agent binary in the same image runs.
	register("beszel", func(t *T) {
		const base = "http://127.0.0.1:18090"
		u := t.User("1001")
		t.Contains(t.RunAny("--version"), t.Want, "beszel --version")
		t.Contains(t.RunEntryAny("/usr/bin/beszel-agent", "--version"), t.Want, "beszel-agent --version")
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--tmpfs", "/beszel_data:rw,uid=1001,gid=1001", "-p", "127.0.0.1:18090:8090"})
		t.waitFor(60, time.Second, name, "hub never answered /api/health", func() bool {
			if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
				t.Fatalf("hub exited\n%s", t.Logs(name))
			}
			c, _ := Get(base + "/api/health")
			return c == 200
		})
		_, page := Get(base + "/")
		t.ContainsFold(page, "beszel", "embedded web UI")
		if out, err := docker("exec", name, "/usr/bin/beszel", "superuser", "upsert", "smoke@example.com", "Smoke-Pass-2026"); err != nil {
			t.Fatalf("superuser upsert: %s", out)
		}
		j := map[string]string{"Content-Type": "application/json"}
		_, auth := DoH("POST", base+"/api/collections/_superusers/auth-with-password", `{"identity":"smoke@example.com","password":"Smoke-Pass-2026"}`, j)
		tok := t.Match(auth, `"token":"([^"]+)"`, "superuser sign-in")
		h := map[string]string{"Content-Type": "application/json", "Authorization": tok}
		if c, b := DoH("POST", base+"/api/collections", `{"name":"smoke","type":"base","listRule":"","viewRule":"","createRule":"",
			"fields":[{"name":"f","type":"file","maxSelect":1,"maxSize":1048576,"thumbs":["8x8"]}]}`, h); c >= 300 {
			t.Fatalf("create a collection with a thumbnailed file field: %d %s", c, b)
		}
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		fw, _ := mw.CreateFormFile("f", "evil.png")
		fw.Write(paletteOverflowBMP())
		mw.Close()
		req, _ := http.NewRequest("POST", base+"/api/collections/smoke/records", &body)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("upload: %v", err)
		}
		rb, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var rec struct{ ID, F string }
		if json.Unmarshal(rb, &rec) != nil || rec.ID == "" || rec.F == "" {
			t.Fatalf("upload the crafted file: %d %s", resp.StatusCode, rb)
		}
		// the thumbnail request decodes the upload with imaging
		Get(base + "/api/files/smoke/" + rec.ID + "/" + rec.F + "?thumb=8x8")
		time.Sleep(2 * time.Second)
		if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
			t.Fatalf("the hub died thumbnailing the crafted image\n%s", t.Logs(name))
		}
		if c, _ := Get(base + "/api/health"); c != 200 {
			t.Fatalf("the hub stopped serving after the crafted thumbnail")
		}
		t.Logf("smoke test passed (beszel %s: read-only hub, superuser sign-in, crafted BMP thumbnail survived, agent runs; user %s)", t.Want, u)
	})
}
