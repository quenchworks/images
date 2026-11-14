package smoke

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

func init() {
	// A shell-less base: its rootfs is checked from outside (no shell, no apk, CA bundle, tzdata, a
	// uid 1001 passwd entry), then a static binary layered onto it runs read-only as uid 1001.
	register("static", func(t *T) {
		u := t.User("1001")
		files := t.Files("")
		has := func(p string) bool { return slices.Contains(files, p) || slices.Contains(files, "./"+p) }
		for _, sh := range []string{"bin/sh", "bin/bash", "bin/ash", "bin/dash", "bin/busybox", "usr/bin/sh", "usr/bin/bash", "usr/bin/busybox", "sbin/apk", "usr/bin/apk", "usr/sbin/apk"} {
			if has(sh) {
				t.Fatalf("%s is present in the static base", sh)
			}
		}
		for _, f := range []string{"etc/ssl/certs/ca-certificates.crt", "usr/share/zoneinfo/UTC"} {
			if !has(f) {
				t.Fatalf("%s missing", f)
			}
		}
		dir := t.TempDir()
		id := t.Docker("create", t.Image, "/nonexistent")
		t.Cleanup(func() { docker("rm", id) })
		t.Docker("cp", id+":/etc/passwd", filepath.Join(dir, "passwd"))
		pw, _ := os.ReadFile(filepath.Join(dir, "passwd"))
		t.Match(string(pw), `:x?:1001:1001:`, "uid 1001 in /etc/passwd")

		// a fully static binary built with musl in a throwaway builder, layered onto the base
		os.WriteFile(filepath.Join(dir, "hello.c"), []byte("#include <unistd.h>\n#include <stdio.h>\nint main(void) { printf(\"quench-static-ok uid=%d\\n\", (int)getuid()); return 0; }\n"), 0o644)
		os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM alpine:3 AS sbuild\nRUN apk add --no-cache musl-dev gcc\nCOPY hello.c /hello.c\nRUN gcc -static -O2 -o /hello /hello.c && /hello\n\nFROM "+t.Image+"\nCOPY --from=sbuild /hello /hello\nENTRYPOINT [\"/hello\"]\n"), 0o644)
		tag := t.Name("hello") + ":local"
		t.Docker("build", "-t", tag, dir)
		t.Cleanup(func() { docker("rmi", "-f", tag) })
		out := t.Docker("run", "--rm", "--read-only", "--tmpfs", "/tmp", tag)
		if !strings.HasPrefix(out, "quench-static-ok uid=1001") {
			t.Fatalf("static binary did not run as uid 1001 on a read-only rootfs: %q", out)
		}
		t.Logf("smoke test passed (static: shell-less, no apk, CA bundle and tzdata, a static binary runs read-only; user %s)", u)
	})

	// Garage on a read-only rootfs with tmpfs data: the single-node layout bootstraps from env, then an
	// S3 bucket and object roundtrip, and the garage CLI lists the bucket.
	register("garage", func(t *T) {
		const ak = "GK31c93ec1a0a0a0a0a0a0a0a0" // GK + exactly 24 hex characters
		const sk = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
		u := t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/data:rw,mode=1777", "--tmpfs", "/tmp:rw,mode=1777",
			"-e", "GARAGE_RPC_SECRET=" + hexRand(32), "-e", "GARAGE_ADMIN_TOKEN=quenchadmintoken",
			"-e", "GARAGE_S3_ACCESS_KEY=" + ak, "-e", "GARAGE_S3_SECRET_KEY=" + sk, "-e", "GARAGE_DEFAULT_BUCKET=gate",
			"-p", "127.0.0.1:13900:3900", "-p", "127.0.0.1:13903:3903"})
		t.waitFor(60, time.Second, name, "garage admin /health did not become healthy", func() bool {
			if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
				t.Fatalf("container exited\n%s", t.Logs(name))
			}
			c, _ := Get("http://127.0.0.1:13903/health")
			return c == 200
		})
		// layout apply, key import and bucket create finish after /health
		t.WaitLog(name, "garage: ready", 30)
		t.s3RoundTrip("garage", name, "http://127.0.0.1:13900", ak, sk, "gate")
		out, _ := docker("exec", name, "sh", "-c", "GARAGE_CONFIG_FILE=/data/garage.toml garage bucket list")
		t.Match(out, `\bgate\b`, "bucket listed by the garage CLI")
		v, _ := docker("exec", name, "/usr/bin/garage", "--version")
		if t.Want != "" && !regexp.MustCompile(regexp.QuoteMeta(t.Want)).MatchString(v) {
			t.Fatalf("garage --version %q, expected %s", v, t.Want)
		}
		t.Logf("smoke test passed (garage %s: read-only rootfs, layout bootstrap, S3 bucket and object roundtrip; user %s)", t.Want, u)
	})
}
