package smoke

import (
	"archive/tar"
	"compress/gzip"
	"debug/elf"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// untarGz extracts a .tar.gz into dir (regular files, dirs and symlinks).
func untarGz(src, dir string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(zr)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		p := filepath.Join(dir, filepath.Clean("/"+h.Name))
		switch h.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(p, 0o755)
		case tar.TypeSymlink:
			os.MkdirAll(filepath.Dir(p), 0o755)
			os.Symlink(h.Linkname, p)
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(p), 0o755)
			w, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(h.Mode)&0o777|0o200)
			if err != nil {
				return err
			}
			io.Copy(w, tr)
			w.Close()
		}
	}
}

func init() {
	register("cosmian-kms", func(t *T) {
		const b = "http://127.0.0.1:19998"
		u := t.User("1001")
		vol, name := t.Name("state"), t.Name("")
		t.Cleanup(func() { docker("volume", "rm", vol) })
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/var/lib/cosmian", "-p", "19998:9998"})
		v := t.WaitHTTP(b+"/version", 40, name)
		t.Match(strings.Trim(v, `"`), `[0-9]+\.[0-9]+\.[0-9]+`, "version endpoint semver")
		bin := filepath.Join(t.TempDir(), "cosmian_kms")
		t.Docker("cp", name+":/usr/bin/cosmian_kms", bin)
		ef, err := elf.Open(bin)
		if err != nil {
			t.Fatalf("not an ELF binary: %v", err)
		}
		libs, _ := ef.ImportedLibraries()
		ef.Close()
		if !regexp.MustCompile(`libssl`).MatchString(strings.Join(libs, " ")) {
			t.Fatalf("binary does not link libssl dynamically; a private OpenSSL was baked in (%v)", libs)
		}
		c, _ := Post(b+"/kmip/2_1", "application/json", `{"tag":"Query","type":"Structure","value":[]}`)
		if c == 0 {
			t.Fatalf("no response at all from the KMIP endpoint")
		}
		t.Logf("smoke test passed (nonroot %s, /version %s, dynamic libssl, KMIP route answered %d)", u, v, c)
	})
	register("dagger", func(t *T) {
		v := t.RunEntry("/usr/local/bin/dagger", "version")
		if t.Want != "" {
			t.Contains(v, "v"+t.Want, "cli version")
		}
		t.Contains(v, "image://ghcr.io/quenchworks/images/dagger:", "default runner is this image")
		// the bundled Go SDK runtime layer is shipped too, so it is gated like the image
		w := t.TempDir()
		x := t.Name("x")
		t.Docker("create", "--name", x, t.Image)
		t.Cleanup(func() { docker("rm", "-f", x) })
		t.Docker("cp", x+":/usr/local/share/dagger/content", filepath.Join(w, "content"))
		t.Docker("cp", x+":/etc/dagger/go-sdk-manifest-digest", filepath.Join(w, "digest"))
		dig, _ := os.ReadFile(filepath.Join(w, "digest"))
		m := strings.TrimSpace(string(dig))
		m = m[strings.Index(m, ":")+1:]
		mb, err := os.ReadFile(filepath.Join(w, "content/blobs/sha256", m))
		if err != nil {
			t.Fatalf("Go SDK manifest blob: %v", err)
		}
		var man struct{ Layers []struct{ Digest string } }
		if json.Unmarshal(mb, &man) != nil || len(man.Layers) == 0 {
			t.Fatalf("Go SDK manifest unreadable")
		}
		sdk := filepath.Join(w, "sdk")
		if err := untarGz(filepath.Join(w, "content/blobs/sha256", strings.TrimPrefix(man.Layers[0].Digest, "sha256:")), sdk); err != nil {
			t.Fatalf("Go SDK layer: %v", err)
		}
		if st, err := os.Stat(filepath.Join(sdk, "usr/local/bin/codegen")); err != nil || st.Mode()&0o111 == 0 {
			t.Fatalf("Go SDK runtime has no codegen")
		}
		if _, err := os.Lstat(filepath.Join(sdk, "usr/bin/go")); err != nil {
			t.Fatalf("Go SDK runtime has no go")
		}
		if _, err := exec.LookPath("trivy"); err == nil {
			if out, err := runHost("trivy", "rootfs", sdk, "--exit-code", "1", "--ignore-unfixed", "--severity", "CRITICAL,HIGH,MEDIUM,LOW",
				"--scanners", "vuln", "--detection-priority", "comprehensive", "--quiet"); err != nil {
				t.Fatalf("Go SDK runtime has fixable CVEs:\n%s", out)
			}
		} else if os.Getenv("CI") != "" {
			t.Fatalf("trivy is not on PATH in CI; the Go SDK runtime layer would go ungated")
		} else {
			t.Logf("trivy not on PATH; Go SDK runtime layer not gated in this run")
		}
		name := t.Name("")
		t.Start(name, []string{"--privileged"})
		t.Docker("exec", name, "mkdir", "-p", "/tmp/m")
		dx := func(args ...string) (string, error) {
			return docker(append([]string{"exec", "-e", "_EXPERIMENTAL_DAGGER_RUNNER_HOST=unix:///run/dagger/engine.sock", "-w", "/tmp/m", name, "dagger", "--progress=plain"}, args...)...)
		}
		t.waitFor(90, 2*time.Second, name, "engine did not come up", func() bool { _, err := dx("core", "version"); return err == nil })
		out, _ := dx("-c", "container | from cgr.dev/chainguard/busybox:latest | with-exec -- echo quench | stdout")
		t.Match(out, `(?m)^quench$`, "container through the engine")
		if out, err := dx("init", "--sdk=go", "--name=hello"); err != nil {
			t.Fatalf("dagger init: %s", out)
		}
		out, _ = dx("call", "container-echo", "--string-arg=quench", "stdout")
		t.Match(out, `(?m)^quench$`, "Go module call")
		t.Logf("smoke test passed (%s)", strings.SplitN(v, "\n", 2)[0])
	})
	register("gitea", func(t *T) {
		t.RunEntry("/usr/bin/gitea", "--version")
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw", "--tmpfs", "/data:rw,uid=1001,gid=1001", "--tmpfs", "/etc/gitea:rw,uid=1001,gid=1001",
			"-p", "3000:3000", "-e", "GITEA_APP_INI=/etc/gitea/app.ini", "--entrypoint", "/bin/sh"}, "-c", `cat > /etc/gitea/app.ini <<INI
APP_NAME = QuenchWorks Gitea smoke
RUN_MODE = prod
[server]
HTTP_ADDR = 0.0.0.0
HTTP_PORT = 3000
[log]
MODE = console
LEVEL = info
INI
exec /usr/bin/gitea web --config /etc/gitea/app.ini`)
		t.waitFor(60, time.Second, name, "gitea did not serve /", func() bool {
			if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
				t.Fatalf("container exited early\n%s", t.Logs(name))
			}
			c, _ := Get("http://127.0.0.1:3000/")
			return c == 200
		})
		t.Logf("smoke test passed (nonroot uid: %s, read-only rootfs, install page served without a DB)", t.User("1001"))
	})
}
