package smoke

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// minimalPDF returns a one-page PDF with a text layer holding word (no image: OCR's skip mode keeps the text).
func minimalPDF(word string) []byte {
	content := fmt.Sprintf("BT /F1 24 Tf 72 720 Td (%s) Tj ET", word)
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offs := make([]int, len(objs))
	for i, o := range objs {
		offs[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	x := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, o := range offs {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, x)
	return b.Bytes()
}

func init() {
	// On a read-only rootfs with our postgresql and a password-protected valkey: migrate and manage_superuser run,
	// the web server and a Celery worker start, the admin gets an API token (a wrong password does not), a PDF posted
	// to /api/documents/post_document/ is consumed by the worker and its text is searchable, and a restart keeps it.
	register("paperless-ngx", func(t *T) {
		const base = "http://127.0.0.1:18114"
		const dbpw, kvpw, pw = "smoke-db-pass", "smoke-kv-pass", "Smoke-Paperless-2026!"
		// ordinary words: OCR rasterizes the page, and an invented token can come back with a misread letter
		const word = "archive"
		u := t.User("1001")
		net := t.Name("net")
		t.Docker("network", "create", net)
		t.Cleanup(func() { docker("network", "rm", net) })
		pg, kv := t.Name("pg"), t.Name("kv")
		t.Cleanup(func() { docker("rm", "-f", pg, kv) })
		t.Docker("run", "-d", "--name", pg, "--network", net, "-e", "POSTGRES_PASSWORD="+dbpw, "-e", "POSTGRES_DB=paperless", "ghcr.io/quenchworks/images/postgresql:18.7")
		t.Docker("run", "-d", "--name", kv, "--network", net, "ghcr.io/quenchworks/images/valkey:9.1.3", "--requirepass", kvpw)
		t.waitCount(pg, "ready to accept connections", 2, 60)
		data, media := t.Name("data"), t.Name("media")
		t.Docker("volume", "create", data)
		t.Docker("volume", "create", media)
		t.Cleanup(func() { docker("volume", "rm", data, media) })
		env := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--network", net,
			"-v", data + ":/usr/src/paperless/data", "-v", media + ":/usr/src/paperless/media",
			// the startup checks write-test the consume folder (the chart mounts a volume there)
			"--tmpfs", "/usr/src/paperless/consume:rw,uid=1001,gid=1001",
			"-e", "PAPERLESS_DBHOST=" + pg, "-e", "PAPERLESS_DBUSER=postgres", "-e", "PAPERLESS_DBPASS=" + dbpw, "-e", "PAPERLESS_DBNAME=paperless",
			"-e", "PAPERLESS_REDIS=redis://:" + kvpw + "@" + kv + ":6379", "-e", "PAPERLESS_SECRET_KEY=quench-smoke-secret-0123456789abcdef"}
		oneShot := func(label string, extra []string, args ...string) {
			n := t.Name(label)
			t.Cleanup(func() { docker("rm", "-f", n) })
			out, err := docker(append(append(append([]string{"run", "--name", n, "--user", "1001", "--entrypoint", "/usr/src/paperless/venv/bin/python"}, env...), extra...), append([]string{t.Image, "manage.py"}, args...)...)...)
			if err != nil {
				t.Fatalf("manage.py %s: %v\n%s", strings.Join(args, " "), err, out)
			}
		}
		oneShot("migrate", nil, "migrate", "--skip-checks", "--no-input")
		// creates data/index: a process that finds no index dir opens an in-memory one and its writes are lost
		oneShot("index", nil, "document_index", "reindex", "--if-needed", "--no-progress-bar")
		oneShot("su", []string{"-e", "PAPERLESS_ADMIN_USER=smoke", "-e", "PAPERLESS_ADMIN_PASSWORD=" + pw}, "manage_superuser")
		worker := t.Name("worker")
		t.Start(worker, append(append([]string{}, env...), "--entrypoint", "/usr/src/paperless/venv/bin/celery"), "--skip-checks", "--app", "paperless", "worker", "--loglevel", "INFO", "--without-mingle", "--without-gossip")
		web := func(name string) {
			t.Start(name, append(append([]string{}, env...), "-p", "127.0.0.1:18114:8000"))
			t.WaitHTTP(base+"/accounts/login/", 120, name)
		}
		name := t.Name("")
		web(name)
		j := map[string]string{"Content-Type": "application/json"}
		token := func(pass string) (int, string) {
			c, b := DoH("POST", base+"/api/token/", `{"username":"smoke","password":"`+pass+`"}`, j)
			var r struct{ Token string }
			json.Unmarshal([]byte(b), &r)
			return c, r.Token
		}
		c, tok := token(pw)
		if c != http.StatusOK || tok == "" {
			t.Fatalf("API token: %d\n%s", c, t.Logs(name))
		}
		auth := map[string]string{"Authorization": "Token " + tok}
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		fw, _ := mw.CreateFormFile("document", "smoke.pdf")
		fw.Write(minimalPDF("Quench " + word + " document"))
		mw.WriteField("title", "Quench smoke")
		mw.Close()
		req, _ := http.NewRequest("POST", base+"/api/documents/post_document/", &body)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("Authorization", "Token "+tok)
		r, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("post_document: %v", err)
		}
		pb, _ := io.ReadAll(r.Body)
		r.Body.Close()
		if r.StatusCode != http.StatusOK {
			t.Fatalf("post_document: %d %s", r.StatusCode, pb)
		}
		// consumed: the document exists and its extracted content holds the word
		consumed := func() (bool, string) {
			last := ""
			for i := 0; i < 90; i++ {
				c, b := DoH("GET", base+"/api/documents/", "", auth)
				last = fmt.Sprintf("%d %s", c, b)
				if c == http.StatusOK && strings.Contains(b, word) {
					return true, ""
				}
				time.Sleep(2 * time.Second)
			}
			return false, last
		}
		// searchable: full-text search (the tantivy index) finds it
		searchable := func() (bool, string) {
			last := ""
			for i := 0; i < 30; i++ {
				c, b := DoH("GET", base+"/api/documents/?query="+word, "", auth)
				last = fmt.Sprintf("%d %s", c, b)
				if c == http.StatusOK && strings.Contains(b, word) {
					return true, ""
				}
				time.Sleep(2 * time.Second)
			}
			return false, last
		}
		found := func() bool {
			if ok, last := consumed(); !ok {
				t.Fatalf("the worker did not consume the PDF: %s", last[:min(600, len(last))])
			}
			if ok, last := searchable(); !ok {
				t.Fatalf("the document is stored but full-text search does not find it: %s\n%s", last[:min(400, len(last))], t.Logs(worker))
			}
			return true
		}
		found()
		if c, tok := token("wrong-password-1"); c == http.StatusOK || tok != "" {
			t.Fatalf("a wrong password got a token (%d)", c)
		}
		docker("rm", "-f", name)
		web(t.Name("again"))
		found()
		t.Logf("smoke test passed (paperless-ngx %s: read-only boot, migrate + document_index + manage_superuser, web + Celery worker on postgresql and valkey, API token, PDF consumed and searchable, wrong password refused, kept across a restart; user %s)", t.Want, u)
	})
}
