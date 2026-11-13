package probe

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFakeAPI(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "ns.json"), []byte(`{"kind":"Namespace"}`), 0o644)
	port, patch, log := filepath.Join(d, "port"), filepath.Join(d, "patch.json"), filepath.Join(d, "api.log")
	go FakeAPI([]string{"--port-file", port, "--log", log,
		"--route", "GET /api/v1/namespaces/x=200:" + filepath.Join(d, "ns.json"),
		"--route", "PATCH /api/v1/namespaces/x=200:" + filepath.Join(d, "ns.json"),
		"--record", "PATCH /api/v1/namespaces/x=" + patch})
	var base string
	for i := 0; i < 50 && base == ""; i++ {
		if b, _ := os.ReadFile(port); len(b) > 0 {
			base = "http://127.0.0.1:" + string(b)
		}
		time.Sleep(20 * time.Millisecond)
	}
	get := func(m, p, body string) (int, string) {
		req, _ := http.NewRequest(m, base+p, strings.NewReader(body))
		r, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(r.Body)
		return r.StatusCode, string(b)
	}
	if c, b := get("GET", "/api/v1/namespaces/x?watch=0", ""); c != 200 || b != `{"kind":"Namespace"}` {
		t.Fatalf("route: %d %s", c, b)
	}
	if c, b := get("GET", "/nope", ""); c != 404 || !strings.Contains(b, "NotFound") {
		t.Fatalf("unrouted: %d %s", c, b)
	}
	get("PATCH", "/api/v1/namespaces/x", `[{"op":"add"}]`)
	if b, _ := os.ReadFile(patch); string(b) != `[{"op":"add"}]` {
		t.Fatalf("record: %q", b)
	}
	if b, _ := os.ReadFile(log); !strings.Contains(string(b), "/nope") {
		t.Fatalf("log: %q", b)
	}
}
