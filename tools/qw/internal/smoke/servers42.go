package smoke

import (
	"strings"
	"time"
)

func init() {
	register("qdrant", func(t *T) {
		const b = "http://127.0.0.1:6333"
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/qdrant:rw,mode=1777", "--tmpfs", "/tmp:rw,mode=1777", "-p", "127.0.0.1:6333:6333", "-p", "127.0.0.1:6334:6334"})
		t.waitFor(60, time.Second, name, "qdrant /healthz did not become healthy", func() bool {
			if s := t.Docker("inspect", "-f", "{{.State.Status}}", name); s != "running" {
				t.Fatalf("container exited\n%s", t.Logs(name))
			}
			c, _ := Get(b + "/healthz")
			return c == 200
		})
		_, root := Get(b + "/")
		t.Contains(root, `"version"`, "root version JSON")
		for _, p := range []string{"/livez", "/readyz", "/dashboard/"} {
			if c, _ := Get(b + p); c != 200 {
				t.Fatalf("%s returned %d", p, c)
			}
		}
		_, cols := Get(b + "/collections")
		t.Contains(cols, `"result"`, "/collections")
		_, cr := DoH("PUT", b+"/collections/gate", `{"vectors":{"size":4,"distance":"Dot"}}`, map[string]string{"Content-Type": "application/json"})
		t.Contains(cr, `"result":true`, "create collection")
		_, cols = Get(b + "/collections")
		t.Contains(cols, `"gate"`, "collection listed")
		t.Logf("%s", t.Docker("exec", name, "/usr/bin/qdrant", "--version"))
		t.Logf("smoke test passed (nonroot user: %s, read-only rootfs, probes, collection round trip, dashboard)", t.User("1001"))
	})
	register("quickwit", func(t *T) {
		const b = "http://127.0.0.1:7280"
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/quickwit/qwdata:rw,mode=1777", "--tmpfs", "/tmp:rw,mode=1777", "-p", "127.0.0.1:7280:7280", "-p", "127.0.0.1:7281:7281"})
		t.waitFor(90, time.Second, name, "quickwit /health/livez did not become healthy", func() bool {
			if s := t.Docker("inspect", "-f", "{{.State.Status}}", name); s != "running" {
				t.Fatalf("container exited\n%s", t.Logs(name))
			}
			c, _ := Get(b + "/health/livez")
			return c == 200
		})
		t.WaitHTTP(b+"/health/readyz", 60, name)
		_, v := Get(b + "/api/v1/version")
		t.Contains(v, `"build"`, "version endpoint")
		_, idx := Get(b + "/api/v1/indexes")
		t.Contains(idx, "[", "index list")
		if c, _ := Get(b + "/ui/search"); c != 200 {
			t.Fatalf("/ui not 200 (got %d)", c)
		}
		t.Logf("%s", t.Docker("exec", name, "/usr/bin/quickwit", "--version"))
		t.Logf("smoke test passed (nonroot user: %s, health, version, index list, UI)", t.User("1001"))
	})
	register("typesense", func(t *T) {
		const b, key = "http://localhost:18108", "smoke-key-123"
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/data:rw,mode=1777,exec", "--tmpfs", "/tmp:rw,mode=1777", "-p", "18108:8108", "-e", "TYPESENSE_API_KEY=" + key})
		if uid := t.Docker("exec", name, "id", "-u"); uid != "1001" {
			t.Fatalf("not running as uid 1001 (%s)", uid)
		}
		t.waitFor(60, time.Second, name, "typesense did not become healthy", func() bool { _, h := Get(b + "/health"); return strings.Contains(h, `"ok":true`) })
		h := map[string]string{"X-TYPESENSE-API-KEY": key, "Content-Type": "application/json"}
		if c, r := DoH("POST", b+"/collections", `{"name":"books","fields":[{"name":"title","type":"string"}]}`, h); c/100 != 2 {
			t.Fatalf("create collection: %d %s", c, r)
		}
		if c, r := DoH("POST", b+"/collections/books/documents", `{"id":"1","title":"The Hitchhiker Guide"}`, h); c/100 != 2 {
			t.Fatalf("index document: %d %s", c, r)
		}
		_, s := DoH("GET", b+"/collections/books/documents/search?q=hitchiker&query_by=title", "", h)
		t.Contains(s, `"found":1`, "typo-tolerant search")
		if c, _ := Get(b + "/collections"); c/100 == 2 {
			t.Fatalf("request succeeded without the API key")
		}
		t.Logf("smoke test passed (uid 1001, collection, document, typo-tolerant search, API key enforced)")
	})
}
