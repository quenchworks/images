package smoke

import (
	"bytes"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

func init() {
	// Against PostgreSQL: the one-off --install creates the schema, an admin and an API user,
	// the server boots on a read-only rootfs and serves /api/health and the embedded admin UI,
	// a valid PNG uploads with a thumbnail (imaging), and the palette-overflow BMP uploaded as
	// evil.png is refused while the server keeps serving (CVE-2023-36308 mitigation).
	register("listmonk", func(t *T) {
		const base = "http://127.0.0.1:19000"
		u := t.User("1001")
		t.Contains(t.RunAny("--version"), t.Want, "listmonk --version")
		net_, pg, name := t.Name("net"), t.Name("pg"), t.Name("")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Cleanup(func() { docker("rm", "-f", pg) })
		// superuser postgres, database listmonk: our postgres image creates POSTGRES_DB only
		// when it differs from the user
		t.Docker("run", "-d", "--name", pg, "--network", net_, "-e", "POSTGRES_PASSWORD=pw", "-e", "POSTGRES_DB=listmonk",
			envOr("PG_IMAGE", "ghcr.io/quenchworks/images/postgresql:18.6"))
		t.waitFor(60, time.Second, pg, "postgres never accepted a query", func() bool {
			_, err := docker("exec", "-e", "PGPASSWORD=pw", pg, "psql", "-h", "127.0.0.1", "-U", "postgres", "-d", "listmonk", "-tAc", "select 1")
			return err == nil
		})
		db := []string{"--network", net_, "-e", "LISTMONK_db__host=" + pg, "-e", "LISTMONK_db__port=5432", "-e", "LISTMONK_db__user=postgres",
			"-e", "LISTMONK_db__password=pw", "-e", "LISTMONK_db__database=listmonk", "-e", "LISTMONK_db__ssl_mode=disable"}
		out, err := t.RunFlags(append(append([]string{}, db...), "-e", "LISTMONK_ADMIN_USER=admin", "-e", "LISTMONK_ADMIN_PASSWORD=Smoke-Admin-2026",
			"-e", "LISTMONK_ADMIN_API_USER=smoke"), "--config=", "--install", "--idempotent", "--yes")
		if err != nil {
			t.Fatalf("listmonk --install: %v\n%s", err, out)
		}
		tok := t.Match(out, `LISTMONK_ADMIN_API_TOKEN="([^"]+)"`, "API token from --install")
		auth := "token smoke:" + tok

		t.Start(name, append([]string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--tmpfs", "/listmonk/uploads:rw,uid=1001,gid=1001",
			"-p", "127.0.0.1:19000:9000"}, db...))
		t.waitFor(60, time.Second, name, "/api/health never answered 200", func() bool {
			if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
				t.Fatalf("listmonk exited\n%s", t.Logs(name))
			}
			// /api/health needs auth since listmonk v4
			c, _ := DoH("GET", base+"/api/health", "", map[string]string{"Authorization": auth})
			return c == 200
		})
		_, admin := Get(base + "/admin/")
		t.ContainsFold(admin, "listmonk", "embedded admin UI")
		if c, cfg := DoH("GET", base+"/api/config", "", map[string]string{"Authorization": auth}); c != 200 || !strings.Contains(cfg, `"version"`) {
			t.Fatalf("API with the install token: %d %.300s", c, cfg)
		}

		upload := func(fname string, b []byte) (int, string) {
			var body bytes.Buffer
			mw := multipart.NewWriter(&body)
			fw, _ := mw.CreateFormFile("file", fname)
			fw.Write(b)
			mw.Close()
			req, _ := http.NewRequest("POST", base+"/api/media", &body)
			req.Header.Set("Content-Type", mw.FormDataContentType())
			req.Header.Set("Authorization", auth)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return 0, err.Error()
			}
			defer resp.Body.Close()
			rb, _ := io.ReadAll(resp.Body)
			return resp.StatusCode, string(rb)
		}
		var good bytes.Buffer
		png.Encode(&good, image.NewNRGBA(image.Rect(0, 0, 40, 30)))
		if c, b := upload("good.png", good.Bytes()); c != 200 || !strings.Contains(b, "thumb") {
			t.Fatalf("valid PNG upload: %d %s\n%s", c, b, t.Logs(name))
		}
		c, b := upload("evil.png", paletteOverflowBMP())
		time.Sleep(2 * time.Second)
		if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
			t.Fatalf("listmonk died on the crafted image\n%s", t.Logs(name))
		}
		if c == 200 {
			t.Fatalf("the crafted image was accepted: %s", b)
		}
		if c, _ := DoH("GET", base+"/api/health", "", map[string]string{"Authorization": auth}); c != 200 {
			t.Fatalf("listmonk stopped serving after the crafted image")
		}
		t.Logf("smoke test passed (listmonk %s: install + API token, read-only boot, admin UI, PNG thumbnail, crafted BMP refused (HTTP %d) with the server up; user %s)", t.Want, c, u)
	})
}
