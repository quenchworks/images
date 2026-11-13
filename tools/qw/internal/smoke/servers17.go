package smoke

import (
	"archive/tar"
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// selfSignedPEM returns a throwaway self-signed CA certificate in PEM.
func selfSignedPEM(cn string) []byte {
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: cn},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour), IsCA: true, BasicConstraintsValid: true}
	der, _ := x509.CreateCertificate(rand.Reader, tpl, tpl, &k.PublicKey, k)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

// tarFile returns the contents of name inside a tar archive.
func tarFile(path, name string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	tr := tar.NewReader(f)
	for {
		h, err := tr.Next()
		if err != nil {
			return ""
		}
		if strings.TrimPrefix(h.Name, "./") == name {
			b, _ := io.ReadAll(tr)
			return string(b)
		}
	}
}

func init() {
	register("crane", func(t *T) {
		u := t.User("1001")
		if v := t.RunAny("version"); t.Want != "" {
			t.Contains(v, t.Want, "version")
		}
		net_, reg := t.Name("net"), t.Name("reg")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Cleanup(func() { docker("rm", "-f", reg) })
		t.Docker("run", "-d", "--name", reg, "--network", net_, "--tmpfs", "/var/lib/registry:uid=1001,gid=1001", "ghcr.io/quenchworks/images/distribution:3.1.2")
		w := t.TempDir()
		var buf bytes.Buffer
		tw := tar.NewWriter(&buf)
		body := []byte("quenchworks-crane-smoke\n")
		tw.WriteHeader(&tar.Header{Name: "etc/", Typeflag: tar.TypeDir, Mode: 0o755})
		tw.WriteHeader(&tar.Header{Name: "etc/smoke.txt", Mode: 0o644, Size: int64(len(body))})
		tw.Write(body)
		tw.Close()
		os.WriteFile(filepath.Join(w, "layer.tar"), buf.Bytes(), 0o644)
		flags := []string{"--network", net_, "--read-only", "--tmpfs", "/tmp", "--tmpfs", "/home/nonroot:uid=1001,gid=1001", "-v", w + ":/work", "-w", "/work"}
		ref := reg + ":5000/smoke/img:v1"
		t.waitFor(30, time.Second, reg, "append failed", func() bool {
			_, err := t.RunFlags(flags, "append", "--insecure", "-f", "layer.tar", "-t", ref)
			return err == nil
		})
		d, _ := t.RunFlags(flags, "digest", "--insecure", ref)
		t.Match(d, `^sha256:[0-9a-f]{64}$`, "digest")
		cat, _ := t.RunFlags(flags, "catalog", "--insecure", reg+":5000")
		t.Match(cat, `(?m)^smoke/img$`, "repository in catalog")
		if out, err := t.RunFlags(flags, "export", "--insecure", ref, "out.tar"); err != nil {
			t.Fatalf("export: %s", out)
		}
		if got := tarFile(filepath.Join(w, "out.tar"), "etc/smoke.txt"); got != string(body) {
			t.Fatalf("exported file missing or wrong: %q", got)
		}
		t.Logf("smoke test passed (uid %s, append + digest + export round trip)", u)
	})
	register("erlang", func(t *T) {
		if t.Want == "" {
			t.Fatalf("usage: qw smoke erlang <image-ref> <major>")
		}
		major := strings.SplitN(t.Want, ".", 2)[0]
		ro := []string{"--read-only", "--tmpfs", "/tmp"}
		erl := func(expr string) (string, error) { return t.RunFlags(ro, "-noshell", "-eval", expr) }
		if otp, _ := erl(`io:format("~s", [erlang:system_info(otp_release)]), halt().`); otp != major {
			t.Fatalf("expected OTP release %s, got '%s'", major, otp)
		}
		if out, _ := erl(`io:format("eval-~p", [1 + 1]), halt().`); out != "eval-2" {
			t.Fatalf("eval output wrong: '%s'", out)
		}
		if uid, _ := erl(`io:format("~s", [string:trim(os:cmd("id -u"))]), halt().`); uid != "1001" {
			t.Fatalf("expected uid 1001, got '%s'", uid)
		}
		t.User("1001")
		w := t.TempDir()
		os.WriteFile(filepath.Join(w, "h.escript"), []byte("#!/usr/bin/env escript\nmain(_) -> io:format(\"quench-erlang~n\").\n"), 0o644)
		if out, _ := t.RunFlags(append(ro, "-v", w+":/work:ro", "-w", "/work", "--entrypoint", "/usr/bin/escript"), "h.escript"); out != "quench-erlang" {
			t.Fatalf("escript output wrong: '%s'", out)
		}
		if _, err := erl(`case file:write_file("/should-fail", <<"x">>) of ok -> halt(0); _ -> halt(1) end.`); err == nil {
			t.Fatalf("rootfs was writable, expected read-only")
		}
		t.Logf("smoke test passed (Erlang/OTP %s, read-only rootfs)", major)
	})
	register("krakend", func(t *T) {
		name, h := t.httpUp([]string{"-p", "127.0.0.1:8080:8080"}, "http://127.0.0.1:8080/__health", 30)
		t.Contains(h, `"status":"ok"`, "health")
		if c, _ := Get("http://127.0.0.1:8080/no-such-endpoint"); c != 404 {
			t.Fatalf("expected 404 on an undeclared route, got %d\n%s", c, t.Logs(name))
		}
		m := regexp.MustCompile(`(?m)^KrakenD Version: (\S+)`).FindStringSubmatch(t.RunEntryAny("/usr/bin/krakend", "version"))
		if m == nil || m[1] == "undefined" || m[1] == "0.0.0" || (t.Want != "" && m[1] != t.Want) {
			t.Fatalf("version %v, expected %s", m, t.Want)
		}
		t.Contains(t.RunEntryAny("/usr/bin/krakend", "check", "-c", "/etc/krakend/krakend.json"), "Syntax OK", "baked config passes check")
		t.Logf("smoke test passed (version %s, nonroot user: %s)", m[1], t.User("1001"))
	})
	register("kured", func(t *T) {
		u := t.User("1001")
		sa := t.TempDir()
		d := filepath.Join(sa, "run/secrets/kubernetes.io/serviceaccount")
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(d, "token"), []byte("smoke\n"), 0o644)
		os.WriteFile(filepath.Join(d, "namespace"), []byte("default\n"), 0o644)
		os.WriteFile(filepath.Join(d, "ca.crt"), selfSignedPEM("smoke"), 0o644)
		os.MkdirAll(filepath.Join(sa, "tmp"), 0o755)
		os.WriteFile(filepath.Join(sa, "tmp/reboot-required"), nil, 0o644)
		name := t.Name("")
		t.Docker("create", "--name", name, "-p", "127.0.0.1:18081:8080", "-e", "KURED_NODE_ID=smoke", "-e", "KUBERNETES_SERVICE_HOST=127.0.0.1",
			"-e", "KUBERNETES_SERVICE_PORT=1", t.Image, "--period=2s", "--reboot-sentinel=/tmp/reboot-required")
		t.Cleanup(func() { docker("rm", "-f", name) })
		t.Docker("cp", sa+"/.", name+":/")
		t.Docker("start", name)
		re := regexp.MustCompile(`(?m)^kured_reboot_required\{node="smoke"\} (\S+)`)
		var m []string
		t.waitFor(30, time.Second, name, "no kured_reboot_required metric", func() bool {
			if s := t.Docker("inspect", "-f", "{{.State.Status}}", name); s != "running" {
				t.Fatalf("kured exited\n%s", t.Logs(name))
			}
			_, b := Get("http://127.0.0.1:18081/metrics")
			m = re.FindStringSubmatch(b)
			return m != nil
		})
		if m[1] != "1" {
			t.Fatalf("sentinel not detected, metric %s", m[0])
		}
		want := "[0-9]"
		if t.Want != "" {
			want = regexp.QuoteMeta(t.Want)
		}
		t.Match(t.Logs(name), "Kubernetes Reboot Daemon: "+want, "version line")
		t.RunEntry("/usr/bin/nsenter", "--version")
		t.Logf("smoke test passed (uid %s, sentinel detected, nsenter present)", u)
	})
	register("promxy", func(t *T) {
		u := t.User("1001")
		if v := t.RunAny("--version"); t.Want != "" {
			t.Contains(v, "version "+t.Want, "version")
		}
		net_, prom, name := t.Name("net"), t.Name("prom"), t.Name("")
		w := t.TempDir()
		os.WriteFile(filepath.Join(w, "promxy.yaml"), []byte("promxy:\n  server_groups:\n    - static_configs:\n        - targets: [\""+prom+":9090\"]\n"), 0o644)
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Cleanup(func() { docker("rm", "-f", prom) })
		t.Docker("run", "-d", "--name", prom, "--network", net_, "--tmpfs", "/prometheus:uid=1001,gid=1001", "ghcr.io/quenchworks/images/prometheus:3.16.0")
		t.Start(name, []string{"--network", net_, "--read-only", "-v", w + ":/c:ro", "-p", "127.0.0.1:18082:8082"}, "--config=/c/promxy.yaml", "--bind-addr=:8082")
		var out string
		t.waitFor(60, 2*time.Second, name, "up not returned through promxy", func() bool {
			_, out = Get("http://127.0.0.1:18082/api/v1/query?query=up")
			return strings.Contains(out, `"__name__":"up"`) && strings.Contains(out, `"status":"success"`)
		})
		if regexp.MustCompile(`panic|read-only file system`).MatchString(t.Logs(name)) {
			t.Fatalf("crashed or wrote to the root\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed: PromQL through promxy reaches the Prometheus server group, user %s", u)
	})
	register("timescaledb", func(t *T) {
		name := t.Name("")
		t.Start(name, []string{"-e", "POSTGRES_PASSWORD=quench", "-p", "127.0.0.1:5432:5432"})
		t.waitFor(60, 2*time.Second, name, "postgres did not become ready", func() bool {
			_, err := docker("exec", name, "pg_isready", "-h", "127.0.0.1", "-q")
			return err == nil
		})
		run := func(q string) string {
			out, err := docker("exec", "-e", "PGPASSWORD=quench", name, "psql", "-h", "127.0.0.1", "-U", "postgres", "-d", "postgres", "-tAc", q)
			if err != nil {
				t.Fatalf("%s: %s", q, out)
			}
			return out
		}
		t.Match(run("SELECT extname FROM pg_extension ORDER BY extname;"), `(?m)^timescaledb$`, "timescaledb extension")
		if v := run("SELECT extversion FROM pg_extension WHERE extname='timescaledb';"); v == "" {
			t.Fatalf("could not read timescaledb extversion")
		}
		run("CREATE TABLE conditions (time TIMESTAMPTZ NOT NULL, device TEXT, temp DOUBLE PRECISION);")
		run("SELECT create_hypertable('conditions','time');")
		run("INSERT INTO conditions VALUES (now(),'dev1',21.5);")
		if got := run("SELECT count(*) FROM conditions;"); got != "1" {
			t.Fatalf("hypertable insert/read failed: %s", got)
		}
		t.Logf("smoke test passed (hypertable round trip; nonroot user: %s)", t.User("1001"))
	})
	register("tusd", func(t *T) {
		u := t.User("1001")
		if v := t.RunAny("-version"); t.Want != "" {
			t.Contains(v, "v"+t.Want, "version")
		}
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/data:uid=1001,gid=1001", "-p", "127.0.0.1:18080:8080"}, "-upload-dir", "/data", "-port", "8080")
		const b = "http://127.0.0.1:18080/files/"
		t.waitFor(30, time.Second, name, "tusd never answered OPTIONS", func() bool { c, _ := Do("OPTIONS", b, ""); return c/100 == 2 })
		blob := make([]byte, 300000)
		rand.Read(blob)
		tus := func(method, url string, hdr map[string]string, body []byte) *http.Response {
			req, _ := http.NewRequest(method, url, bytes.NewReader(body))
			req.Header.Set("Tus-Resumable", "1.0.0")
			for k, v := range hdr {
				req.Header.Set(k, v)
			}
			r, err := httpc.Do(req)
			if err != nil {
				t.Fatalf("%s %s: %v", method, url, err)
			}
			return r
		}
		r := tus("POST", b, map[string]string{"Upload-Length": "300000"}, nil)
		r.Body.Close()
		loc := r.Header.Get("Location")
		if loc == "" {
			t.Fatalf("no upload created (%d)\n%s", r.StatusCode, t.Logs(name))
		}
		if !strings.HasPrefix(loc, "http") {
			loc = "http://127.0.0.1:18080" + loc
		}
		oct := map[string]string{"Content-Type": "application/offset+octet-stream", "Upload-Offset": "0"}
		tus("PATCH", loc, oct, blob[:120000]).Body.Close()
		r = tus("HEAD", loc, nil, nil)
		r.Body.Close()
		off := r.Header.Get("Upload-Offset")
		if off != "120000" {
			t.Fatalf("offset after the first chunk: %s", off)
		}
		oct["Upload-Offset"] = off
		tus("PATCH", loc, oct, blob[120000:]).Body.Close()
		r = tus("GET", loc, nil, nil)
		back, _ := io.ReadAll(r.Body)
		r.Body.Close()
		if !bytes.Equal(back, blob) {
			t.Fatalf("downloaded file differs from the upload (%d bytes)", len(back))
		}
		if regexp.MustCompile(`panic|read-only file system`).MatchString(t.Logs(name)) {
			t.Fatalf("crashed or wrote to the root\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed: two-chunk resumable upload resumed at %s, %s bytes back, user %s", off, strconv.Itoa(len(back)), u)
	})
	register("atlantis", func(t *T) {
		t.RunEntry("/usr/bin/atlantis", "version")
		t.RunEntry("/usr/bin/tofu", "version")
		t.httpUp([]string{"--read-only", "--tmpfs", "/tmp", "--mount", "type=tmpfs,destination=/atlantis-data", "-p", "127.0.0.1:4141:4141"},
			"http://127.0.0.1:4141/healthz", 30,
			"--gh-user=x", "--gh-token=x", "--gh-webhook-secret=x", "--repo-allowlist=*", "--data-dir=/atlantis-data", "--atlantis-url=http://127.0.0.1:4141")
		t.Logf("smoke test passed (nonroot user: %s, read-only rootfs, atlantis + bundled tofu + /healthz)", t.User("1001"))
	})
}
