package smoke

import (
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func init() {
	// Want is the Rust minor (CI passes 1.96, not 1.96.1). The image has no shell, so
	// the crate is written on the host and mounted.
	register("rust", func(t *T) {
		if t.Want == "" {
			t.Fatalf("usage: test.sh <image-ref> <minor>")
		}
		dir := t.TempDir()
		os.MkdirAll(filepath.Join(dir, "src"), 0o777)
		os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]\nname = \"quench_hello\"\nversion = \"0.1.0\"\nedition = \"2021\"\n[[bin]]\nname = \"quench_hello\"\npath = \"src/main.rs\"\n"), 0o666)
		os.WriteFile(filepath.Join(dir, "src/main.rs"), []byte(`fn main() { print!("quench-rust"); }`+"\n"), 0o666)
		ro := []string{"run", "--rm", "--read-only", "--tmpfs", "/tmp:exec", "-e", "CARGO_TARGET_DIR=/tmp/target", "-v", dir + ":/src", "-w", "/src"}
		cv := strings.TrimSpace(t.Docker(append(ro, t.Image, "--version")...))
		t.Logf("%s", cv)
		if !strings.HasPrefix(cv, "cargo "+t.Want+".") {
			t.Fatalf("expected 'cargo %s.*', got %q", t.Want, cv)
		}
		rv := strings.TrimSpace(t.Docker(append(ro, "--entrypoint", "/usr/bin/rustc", t.Image, "--version")...))
		t.Logf("%s", rv)
		if !strings.HasPrefix(rv, "rustc "+t.Want+".") {
			t.Fatalf("expected 'rustc %s.*', got %q", t.Want, rv)
		}
		user := t.User("1001")
		// A zero-dependency crate must build and run --offline on the read-only rootfs.
		if out, err := exec2out(t, append(ro, t.Image, "run", "--quiet", "--offline")...); err != nil || out != "quench-rust" {
			t.Fatalf("cargo run output wrong: %q (%v)", out, err)
		}
		t.Docker(append(ro, t.Image, "build", "--quiet", "--offline")...)
		if _, err := docker("run", "--rm", "--read-only", "--tmpfs", "/tmp:exec", "-e", "CARGO_HOME=/tmp/cargo", "-e", "CARGO_TARGET_DIR=/nope",
			"-v", dir+":/src", "-w", "/src", t.Image, "build", "--quiet", "--offline"); err == nil {
			t.Fatalf("rootfs was writable, expected read-only")
		}
		t.Logf("smoke test passed (Rust %s, configured user: %s, read-only rootfs)", t.Want, user)
	})
	register("varnish", func(t *T) {
		ver := t.Match(t.RunEntryAny("/usr/bin/varnishd", "-V"), `varnish-[0-9]+\.[0-9]+\.[0-9]+`, "varnishd -V")
		t.Logf("%s", ver)
		// The working dir must be an exec tmpfs: varnishd dlopen()s the VCL it compiled there.
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/var/lib/varnish:rw,exec,uid=1001,gid=1001", "--tmpfs", "/tmp:rw,mode=1777",
			"-p", "127.0.0.1:8080:8080", "-p", "127.0.0.1:6082:6082"})
		get := func(path string) (*http.Response, string) {
			r, err := http.Get("http://127.0.0.1:8080" + path)
			if err != nil {
				return nil, ""
			}
			defer r.Body.Close()
			b, _ := io.ReadAll(r.Body)
			return r, string(b)
		}
		var r *http.Response
		var body string
		t.waitFor(45, time.Second, name, "varnish never served a 200", func() bool {
			if s := t.Docker("inspect", "-f", "{{.State.Status}}", name); s != "running" {
				t.Fatalf("container died during startup:\n%s", t.Logs(name))
			}
			r, body = get("/")
			return r != nil && r.StatusCode == 200
		})
		// Only Varnish itself mints X-Varnish ids and the Server: Varnish header.
		xid := r.Header.Get("X-Varnish")
		if xid == "" || !strings.HasPrefix(r.Header.Get("Server"), "Varnish") {
			t.Fatalf("not answered by Varnish: %v", r.Header)
		}
		t.Contains(body, "quench-varnish", "body")
		r2, _ := get("/second")
		if r2 == nil || r2.Header.Get("X-Varnish") == "" || r2.Header.Get("X-Varnish") == xid {
			t.Fatalf("second request did not get its own X-Varnish xid (first was %s)", xid)
		}
		t.Logf("GET / -> 200 with X-Varnish %s, second request %s", xid, r2.Header.Get("X-Varnish"))
		// The CLI greets with "107 <n>", the auth challenge: the manager speaks the admin protocol.
		c, err := net.DialTimeout("tcp", "127.0.0.1:6082", 10*time.Second)
		if err != nil {
			t.Fatalf("varnishadm CLI: %v", err)
		}
		c.SetDeadline(time.Now().Add(10 * time.Second))
		b := make([]byte, 3)
		io.ReadFull(c, b)
		c.Close()
		if string(b) != "107" {
			t.Fatalf("varnishadm CLI did not send the 107 auth challenge (got %q)\n%s", b, t.Logs(name))
		}
		if m := regexp.MustCompile(`(?i)read-only file system|permission denied|C-compiler failed`).FindString(t.Logs(name)); m != "" {
			t.Fatalf("found %q in logs\n%s", m, t.Logs(name))
		}
		t.Logf("smoke test passed (nonroot user: %s, read-only rootfs, cache served HTTP)", t.User("1001"))
	})
	register("vector", func(t *T) {
		dir := t.TempDir()
		cfg := filepath.Join(dir, "vector.yaml")
		os.WriteFile(cfg, []byte(`api:
  enabled: true
  address: "0.0.0.0:8686"
sources:
  demo:
    type: demo_logs
    format: json
    interval: 1
sinks:
  out:
    type: console
    inputs: ["demo"]
    encoding:
      codec: json
`), 0o644)
		name, h := t.httpUp([]string{"--read-only", "--tmpfs", "/vector-data-dir:rw,mode=1700", "--tmpfs", "/tmp:rw,mode=1777",
			"-v", cfg + ":/etc/vector/vector.yaml:ro", "-p", "127.0.0.1:8686:8686"}, "http://127.0.0.1:8686/health", 60)
		t.Contains(h, `"ok"`, "/health payload")
		user := t.User("1001")
		t.Logf("version: %s", strings.TrimSpace(t.Docker("exec", name, "/usr/bin/vector", "--version")))
		t.Logf("smoke test passed (nonroot user: %s, read-only rootfs, api /health 200)", user)
	})
}
