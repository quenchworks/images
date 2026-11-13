package smoke

import (
	"encoding/json"
	"os/exec"
	"strings"
	"time"
)

func init() {
	register("aspnet", func(t *T) {
		if t.Want == "" {
			t.Fatalf("usage: qw smoke aspnet <image-ref> <major>")
		}
		major := strings.SplitN(t.Want, ".", 2)[0]
		ro := []string{"--read-only", "--tmpfs", "/tmp"}
		rts, _ := t.RunFlags(ro, "--list-runtimes")
		t.Contains(rts, "Microsoft.AspNetCore.App "+major+".", "ASP.NET Core runtime")
		t.Contains(rts, "Microsoft.NETCore.App "+major+".", "base .NET runtime")
		v, _ := t.RunFlags(append(append([]string{}, ro...), "--entrypoint", "/usr/bin/dotnet"), "--list-runtimes")
		t.Contains(v, "Microsoft.AspNetCore.App "+major+".", "/usr/bin/dotnet")
		if sdks, _ := t.RunFlags(ro, "--list-sdks"); strings.Contains(sdks, major+".") {
			t.Fatalf("an SDK is installed; this image is not runtime-only")
		}
		if _, err := t.RunFlags(ro, "build", "--help"); err == nil {
			t.Fatalf("'dotnet build' resolved; SDK present")
		}
		info, _ := t.RunFlags(ro, "--info")
		t.Contains(info, "Microsoft.AspNetCore.App "+major+".", "--info")
		t.User("1001")
		t.Contains(t.Docker("inspect", t.Image, "--format", "{{json .Config.Entrypoint}}"), "/usr/bin/dotnet", "entrypoint")
		t.Logf("smoke test passed (ASP.NET Core runtime %s, runtime-only, read-only rootfs)", major)
	})
	register("changedetection", func(t *T) {
		const caddy = "ghcr.io/quenchworks/images/caddy@sha256:6377c513134e78bd41fad0819c9d45830b1868398c1805a5b3ee8494f93072d7"
		const b = "http://127.0.0.1:15000"
		u := t.Docker("inspect", t.Image, "--format", "{{.Config.User}}")
		py := "/opt/changedetection/bin/python"
		// stdout only: the import logs a DEBUG line on stderr
		vb, err := exec.Command("docker", "run", "--rm", "--entrypoint", py, t.Image, "-c", `import sys; sys.path.insert(0,"/app"); import changedetectionio; print(changedetectionio.__version__)`).Output()
		if err != nil {
			t.Fatalf("version: %v", err)
		}
		v := strings.TrimSpace(string(vb))
		if t.Want != "" && v != t.Want {
			t.Fatalf("changedetection.io %s, expected %s", v, t.Want)
		}
		net_, web, name, vol := t.Name("net"), t.Name("web"), t.Name(""), t.Name("data")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Cleanup(func() { docker("rm", "-f", web) })
		t.Docker("run", "-d", "--name", web, "--network", net_, "--network-alias", "web", "--entrypoint", "/usr/bin/caddy", caddy, "respond", "--listen", ":8080", "--body", "quench-watched-page")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		start := func() {
			t.Start(name, []string{"--network", net_, "--read-only", "--tmpfs", "/tmp:uid=1001,gid=1001", "-v", vol + ":/datastore", "-e", "ALLOW_IANA_RESTRICTED_ADDRESSES=true", "-p", "127.0.0.1:15000:5000"})
			t.WaitHTTP(b+"/", 60, name)
		}
		start()
		var key string
		t.waitFor(20, time.Second, name, "no API key in the datastore", func() bool {
			out, err := t.RunFlags([]string{"-v", vol + ":/d", "--entrypoint", py}, "-c", `import json; print(json.load(open("/d/changedetection.json"))["settings"]["application"]["api_access_token"])`)
			key = out
			return err == nil && out != ""
		})
		if c, _ := DoH("GET", b+"/api/v1/watch", "", map[string]string{"x-api-key": "wrong"}); c != 401 && c != 403 {
			t.Fatalf("a wrong API key got HTTP %d", c)
		}
		h := map[string]string{"x-api-key": key, "Content-Type": "application/json"}
		if c, r := DoH("POST", b+"/api/v1/watch", `{"url":"http://web:8080/","title":"quench-smoke-watch"}`, h); c/100 != 2 {
			t.Fatalf("create watch: %d %s", c, r)
		}
		watches := func() string { _, r := DoH("GET", b+"/api/v1/watch", "", h); return r }
		w := watches()
		t.Contains(w, "quench-smoke-watch", "watch listed")
		var all map[string]struct{ URL string }
		json.Unmarshal([]byte(w), &all)
		var uuid string
		for k, v := range all {
			if v.URL == "http://web:8080/" {
				uuid = k
			}
		}
		if uuid == "" {
			t.Fatalf("no watch for http://web:8080/: %s", w)
		}
		t.waitFor(60, 2*time.Second, name, "the page was never fetched", func() bool {
			_, s := DoH("GET", b+"/api/v1/watch/"+uuid+"/history/latest", "", h)
			return strings.Contains(s, "quench-watched-page")
		})
		t.Docker("rm", "-f", name)
		start()
		t.Contains(watches(), "quench-smoke-watch", "watch kept across a restart")
		if strings.Contains(t.Logs(name), "Traceback") || strings.Contains(strings.ToLower(t.Logs(name)), "read-only file system") {
			t.Fatalf("error in the logs\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed (changedetection.io %s: API key, page fetched into a snapshot, watch kept on /datastore; user %s)", v, u)
	})
	register("coolify-helper", func(t *T) {
		want := map[string]string{"mise": "2026.10.14", "railpack": "0.43.0", "nixpacks": "1.41.0", "pack": "0.40.10"}
		name := t.Name("")
		t.Start(name, nil)
		time.Sleep(2 * time.Second)
		if s := t.Docker("inspect", "-f", "{{.State.Status}}", name); s != "running" {
			t.Fatalf("container did not stay up\n%s", t.Logs(name))
		}
		dx := func(args ...string) string { return t.Docker(append([]string{"exec", name}, args...)...) }
		for _, c := range [][]string{{"docker", "--version"}, {"docker", "compose", "version"}, {"docker", "buildx", "version"}, {"mc", "--version"},
			{"git", "--version"}, {"git-lfs", "version"}, {"ssh", "-V"}} {
			dx(c...)
		}
		for tool, v := range want {
			arg := "--version"
			if tool == "pack" {
				arg = "version"
			}
			t.Contains(dx(tool, arg), v, tool+" version")
		}
		t.Contains(strings.ReplaceAll(dx("cat", "/proc/1/cmdline"), "\x00", " "), "tini", "tini as PID 1")
		seed := "/tmp/railpack/mise/mise-" + want["mise"]
		t.Contains(dx(seed, "--version"), want["mise"], "entrypoint seeded railpack's mise")
		if uid := dx("id", "-u"); uid != "0" {
			t.Fatalf("expected root (uid 0) for the privileged helper tier, got %s", uid)
		}
		if u := t.Docker("inspect", t.Image, "--format", "{{.Config.User}}"); u != "" && u != "root" && u != "0" {
			t.Fatalf("image Config.User should be root/empty, got '%s'", u)
		}
		t.Logf("smoke test passed (privileged-by-design root helper; docker, compose, buildx, pack/railpack/nixpacks/mise, mc, git, ssh; tini PID 1; mise seeded)")
	})
}
