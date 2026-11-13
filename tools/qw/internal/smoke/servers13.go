package smoke

import (
	"archive/tar"
	"bytes"
	"crypto/rand"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// helpRuns fails when bin cannot execute (126/127 or an exec error) or prints nothing.
func (t *T) helpRuns(bin string) {
	out, code := t.RunAnyFor(30*time.Second, []string{"--entrypoint", bin}, "--help")
	if code == 126 || code == 127 || out == "" || regexp.MustCompile(`(?i)exec format error|no such file or directory.*exec|permission denied`).MatchString(out) {
		t.Fatalf("%s did not execute (exit %d): %.300s", bin, code, out)
	}
}

func init() {
	register("kfp-frontend", func(t *T) {
		out := t.RunEntryAny("/usr/bin/node", "--input-type=module", "-e", `
import { Client } from "minio";
import { createRequire } from "module";
const require = createRequire("/server/package.json");
const qs = require("query-string");
if (qs.parse("a=%20b").a !== " b") throw new Error("query-string decode");
if (typeof Client !== "function") throw new Error("minio import");
console.log("deps ok");`)
		t.Contains(out, "deps ok", "patched deps load")
		name := t.Name("")
		t.Start(name, []string{"-p", "127.0.0.1::3000"})
		base := "http://127.0.0.1:" + t.hostPort(name, "3000/tcp")
		for i := 0; ; i++ {
			if _, b := Get(base + "/"); strings.Contains(b, `id="root"`) {
				break
			}
			if i == 60 {
				t.Fatalf("UI did not serve index.html\n%s", t.Logs(name))
			}
			time.Sleep(time.Second)
		}
		_, hz := Get(base + "/apis/v1beta1/healthz")
		if t.Want != "" {
			t.Contains(hz, t.Want, "healthz version")
		}
		t.Logf("smoke test passed (healthz: %s)", hz)
	})
	register("kuma", func(t *T) {
		t.Match(t.RunEntryAny("/usr/bin/kuma-cp", "version"), `(?i)Kuma:\s*[0-9]+\.[0-9]+\.[0-9]+`, "kuma-cp version")
		t.RunEntry("/usr/bin/kumactl", "version")
		name, _ := t.httpUp([]string{"-p", "127.0.0.1:5680:5680", "-p", "127.0.0.1:5681:5681"}, "http://127.0.0.1:5680/ready", 60)
		t.WaitHTTP("http://127.0.0.1:5681/", 10, name)
		t.Logf("smoke test passed (nonroot user: %s)", t.User("1001"))
	})
	register("metrics-server", func(t *T) {
		v := t.RunAny("--version")
		t.Match(v, `(?i)v?[0-9]+\.[0-9]+\.[0-9]+`, "version stamped")
		if t.Want != "" {
			t.Contains(v, t.Want, "version")
		}
		boot := t.RunAny("--secure-port=4443", "--cert-dir=/tmp", "--kubelet-insecure-tls")
		t.Contains(boot, "Generated self-signed cert", "reached serving setup")
		t.Contains(boot, "unable to load in-cluster configuration", "exited at the in-cluster config step")
		if m := regexp.MustCompile(`(?i)failed to build open ?api spec|cannot find model definition`).FindString(boot); m != "" {
			t.Fatalf("openapi spec build failed: %s", m)
		}
		t.Logf("smoke test passed (nonroot user: %s; serving is proven by the chart's kind gate)", t.User("1001"))
	})
	register("oras", func(t *T) {
		u := t.User("1001")
		if v := t.Run("version"); t.Want != "" {
			t.Match(v, `Version: *`+regexp.QuoteMeta(t.Want), "version")
		}
		net_, reg := t.Name("net"), t.Name("reg")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Cleanup(func() { docker("rm", "-f", reg) })
		t.Docker("run", "-d", "--name", reg, "--network", net_, "--tmpfs", "/var/lib/registry:uid=1001,gid=1001", "ghcr.io/quenchworks/images/distribution:3.1.2")
		work := t.TempDir()
		blob := make([]byte, 4096)
		rand.Read(blob)
		os.WriteFile(filepath.Join(work, "blob.bin"), blob, 0o644)
		os.Mkdir(filepath.Join(work, "out"), 0o777)
		os.Chmod(filepath.Join(work, "out"), 0o777)
		flags := []string{"--network", net_, "--read-only", "--tmpfs", "/tmp", "--tmpfs", "/home/nonroot:uid=1001,gid=1001", "-v", work + ":/work", "-w", "/work"}
		ref := reg + ":5000/smoke/blob:v1"
		for i := 0; ; i++ {
			out, err := t.RunFlags(flags, "push", "--plain-http", ref, "--artifact-type", "application/vnd.quench.smoke", "blob.bin")
			if err == nil {
				break
			}
			if i == 30 {
				t.Fatalf("push failed: %s\n%s", out, t.Logs(reg))
			}
			time.Sleep(time.Second)
		}
		m, _ := t.RunFlags(flags, "manifest", "fetch", "--plain-http", ref)
		t.Contains(m, "application/vnd.quench.smoke", "artifact type in manifest")
		if out, err := t.RunFlags(flags, "pull", "--plain-http", ref, "-o", "out"); err != nil {
			t.Fatalf("pull: %s", out)
		}
		if got, _ := os.ReadFile(filepath.Join(work, "out/blob.bin")); !bytes.Equal(got, blob) {
			t.Fatalf("pulled file differs")
		}
		t.Logf("smoke test passed (uid %s, push + pull round trip)", u)
	})
	register("postgres-documentdb", func(t *T) {
		name := t.Name("")
		t.Start(name, []string{"-e", "POSTGRES_PASSWORD=quench", "-p", "127.0.0.1:5432:5432"})
		for i := 0; ; i++ {
			if _, err := docker("exec", name, "pg_isready", "-h", "127.0.0.1", "-q"); err == nil {
				break
			}
			if i == 60 {
				t.Fatalf("postgres did not become ready\n%s", t.Logs(name))
			}
			time.Sleep(2 * time.Second)
		}
		run := func(q string) string {
			out, _ := docker("exec", "-e", "PGPASSWORD=quench", name, "psql", "-h", "127.0.0.1", "-U", "postgres", "-d", "postgres", "-tAc", q)
			return out
		}
		exts := "\n" + run("SELECT extname FROM pg_extension ORDER BY extname;") + "\n"
		for _, e := range []string{"documentdb", "documentdb_core", "pg_cron", "vector", "postgis", "rum"} {
			if !strings.Contains(exts, "\n"+e+"\n") {
				t.Fatalf("missing extension: %s (have:%s)", e, exts)
			}
		}
		run(`SELECT documentdb_api.insert_one('smoke','coll','{"_id":1,"hello":"quench"}');`)
		t.Logf("server_version %s", run("SHOW server_version;"))
		t.Logf("smoke test passed (nonroot user: %s)", t.User("1001"))
	})
	register("chisel", func(t *T) {
		u := t.User("1001")
		v := t.Run("--version")
		if t.Want != "" {
			t.Contains(v, t.Want, "version")
		}
		net_, sv, cl, bad, be := t.Name("net"), t.Name("sv"), t.Name("cl"), t.Name("bad"), t.Name("be")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Cleanup(func() { docker("rm", "-f", sv, cl, bad, be) })
		t.Docker("run", "-d", "--name", be, "--network", net_, "--entrypoint", "/usr/bin/caddy", "ghcr.io/quenchworks/images/caddy:2.11.9", "respond", "--listen", ":8000", "quench-backend")
		t.Docker("run", "-d", "--name", sv, "--network", net_, t.Image, "server", "--port", "8080", "--auth", "quench:quench-pass")
		t.Docker("run", "-d", "--name", cl, "--network", net_, "-p", "127.0.0.1::9000", t.Image, "client", "--auth", "quench:quench-pass", "http://"+sv+":8080", "0.0.0.0:9000:"+be+":8000")
		url := "http://127.0.0.1:" + t.hostPort(cl, "9000/tcp") + "/"
		for i := 0; ; i++ {
			if _, out := Get(url); out == "quench-backend" {
				break
			}
			if i == 30 {
				t.Fatalf("tunnel fetch failed\n%s\n%s", t.Logs(sv), t.Logs(cl))
			}
			time.Sleep(time.Second)
		}
		t.Docker("run", "-d", "--name", bad, "--network", net_, t.Image, "client", "--max-retry-count", "1", "--auth", "quench:wrong", "http://"+sv+":8080", "0.0.0.0:9001:"+be+":8000")
		time.Sleep(5 * time.Second)
		t.Match(t.Logs(bad), `(?i)authentication failed|unable to authenticate|handshake failed`, "wrong password refused")
		t.Logf("smoke test passed (%s, nonroot user: %s)", v, u)
	})
	register("kubescape-node-agent", func(t *T) {
		t.helpRuns("/usr/bin/node-agent")
		t.helpRuns("/usr/bin/sbom-scanner")
		if u := t.Docker("inspect", t.Image, "--format", "{{.Config.User}}"); u != "0" {
			t.Fatalf("expected user 0, got '%s'", u)
		}
		if wd := t.Docker("inspect", t.Image, "--format", "{{.Config.WorkingDir}}"); wd != "/root" {
			t.Fatalf("expected WorkingDir /root, got '%s'", wd)
		}
		dir := t.TempDir()
		cid := t.Docker("create", t.Image)
		t.Cleanup(func() { docker("rm", "-f", cid) })
		t.Docker("cp", cid+":/root/tracers.tar", filepath.Join(dir, "tracers.tar"))
		t.Docker("cp", cid+":/root/.ig/config.yaml", filepath.Join(dir, "config.yaml"))
		cfg, _ := os.ReadFile(filepath.Join(dir, "config.yaml"))
		t.Contains(string(cfg), "containerd-socketpath", "ig config")
		f, err := os.Open(filepath.Join(dir, "tracers.tar"))
		if err != nil {
			t.Fatalf("tracers.tar: %v", err)
		}
		defer f.Close()
		var index string
		for tr := tar.NewReader(f); ; {
			h, err := tr.Next()
			if err != nil {
				break
			}
			if h.Name == "index.json" {
				b, _ := io.ReadAll(tr)
				index = string(b)
				break
			}
		}
		for _, r := range []string{"advise_seccomp", "trace_capabilities", "trace_dns", "trace_exec", "trace_open"} {
			t.Contains(index, "gadget/"+r+":v", "tracers.tar index")
		}
		for _, r := range []string{"bpf", "exit", "fork", "hardlink", "http", "iouring_new", "iouring_old", "kmod", "network", "ptrace", "randomx", "ssh", "symlink", "unshare"} {
			t.Match(index, `"`+r+`:latest"|/`+r+`:latest"`, "tracers.tar index "+r)
		}
		t.Logf("smoke test passed (all 19 gadget images indexed)")
	})
	register("kubescape", func(t *T) {
		v := t.RunAny("version")
		if t.Want != "" {
			t.Contains(v, "v"+t.Want, "version")
		}
		t.Run("scan", "--help")
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "deploy.yaml"), []byte(`apiVersion: apps/v1
kind: Deployment
metadata: { name: demo }
spec:
  selector: { matchLabels: { app: demo } }
  template:
    metadata: { labels: { app: demo } }
    spec:
      containers:
        - { name: demo, image: nginx, securityContext: { privileged: true } }
`), 0o644)
		scan, _ := t.RunFlags([]string{"-e", "HOME=/tmp", "--tmpfs", "/tmp", "-v", dir + ":/work:ro"}, "scan", "/work/deploy.yaml", "--format", "json")
		t.Contains(scan, `"controlID"`, "scan control results")
		for _, b := range []string{"ksserver", "downloader"} {
			if _, code := t.RunAnyFor(30*time.Second, []string{"--entrypoint", "/usr/bin/" + b}, "--help"); code == 126 || code == 127 {
				t.Fatalf("/usr/bin/%s did not execute", b)
			}
		}
		t.Logf("smoke test passed")
	})
	register("longhorn-instance-manager", func(t *T) {
		u := t.rootLikeUpstream()
		if v := t.RunEntry("longhorn-instance-manager", "--version"); t.Want != "" {
			t.Contains(v, "v"+t.Want, "version")
		}
		name := t.Name("")
		t.Start(name, []string{"--privileged", "-v", "/dev:/host/dev", "--tmpfs", "/host/sys", "--tmpfs", "/host/lib/modules"}, "instance-manager", "--debug", "daemon", "--listen", ":8500")
		probe := `if command -v instance-manager-liveness-probe >/dev/null; then instance-manager-liveness-probe --data-engine v1 --timeout 2
else for p in 8500 8501 8502 8503; do nc -z -w 2 localhost $p || exit 1; done; fi`
		for i := 0; ; i++ {
			if st := t.Docker("inspect", "-f", "{{.State.Status}}", name); st == "exited" || st == "dead" {
				t.Fatalf("instance-manager exited\n%s", t.Logs(name))
			}
			if _, err := docker("exec", name, "sh", "-c", probe); err == nil {
				break
			}
			if i == 40 {
				out, _ := docker("exec", name, "sh", "-c", probe)
				t.Fatalf("liveness probe never passed: %s\n%s", out, t.Logs(name))
			}
			time.Sleep(time.Second)
		}
		t.Docker("exec", name, "tgtadm", "--lld", "iscsi", "--mode", "target", "--op", "show")
		t.Docker("exec", name, "sg_raw", "-V")
		if _, err := docker("exec", name, "/usr/local/bin/grpc_health_probe", "-addr", "localhost:8500"); err != nil {
			t.Docker("exec", name, "/usr/local/bin/grpc_health_probe", "-version")
		}
		t.Logf("smoke test passed (v1 launcher, liveness probe, tgtd; user %q)", u)
	})
	register("mongodb-exporter", func(t *T) {
		v := t.Run("--version")
		if t.Want != "" {
			t.Contains(v, "Version: "+t.Want, "version")
		}
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-p", "127.0.0.1:9216:9216"}, "--mongodb.uri=mongodb://127.0.0.1:27017")
		slow := &http.Client{Timeout: 25 * time.Second} // a scrape waits on the unreachable mongod
		var m string
		for i := 0; ; i++ {
			if r, err := slow.Get("http://127.0.0.1:9216/metrics"); err == nil {
				b, _ := io.ReadAll(r.Body)
				r.Body.Close()
				if m = string(b); r.StatusCode == 200 {
					break
				}
			}
			if i == 30 {
				t.Fatalf("mongodb_exporter did not serve /metrics\n%s", t.Logs(name))
			}
			time.Sleep(time.Second)
		}
		t.Match(m, `(?m)^mongodb_up`, "mongodb_up on /metrics")
		t.Logf("smoke test passed (nonroot uid: %s, read-only rootfs)", t.User("1001"))
	})
}
