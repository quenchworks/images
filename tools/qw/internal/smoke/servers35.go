package smoke

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func init() {
	register("trino", func(t *T) {
		u := t.Docker("inspect", t.Image, "--format", "{{.Config.User}}")
		name := t.Name("")
		t.Start(name, []string{"-p", "127.0.0.1:8080:8080"})
		var info string
		t.waitFor(90, 2*time.Second, name, "trino did not finish starting", func() bool {
			_, info = Get("http://127.0.0.1:8080/v1/info")
			return strings.Contains(info, `"starting":false`)
		})
		logs := t.Logs(name)
		for _, p := range []string{"iceberg", "postgresql", "tpch"} {
			t.Contains(logs, "-- Finished loading plugin "+p+" --", "plugin "+p)
		}
		if strings.Contains(logs, "Failed to load Brotli native library") {
			t.Fatalf("brotli native library failed to load")
		}
		if t.Want != "" {
			t.Contains(info, `"version":"`+t.Want+`"`, "version")
		}
		h := map[string]string{"X-Trino-User": "smoke"}
		query := func(sql string) string {
			_, r := DoH("POST", "http://127.0.0.1:8080/v1/statement", sql, h)
			var rows [][]any
			for {
				var p struct {
					NextURI string `json:"nextUri"`
					Data    [][]any
					Error   *struct{ Message string }
				}
				if err := json.Unmarshal([]byte(r), &p); err != nil {
					t.Fatalf("%s: bad response %.300s", sql, r)
				}
				if p.Error != nil {
					t.Fatalf("query failed: %s", p.Error.Message)
				}
				rows = append(rows, p.Data...)
				if p.NextURI == "" {
					b, _ := json.Marshal(rows)
					return string(b)
				}
				_, r = DoH("GET", p.NextURI, "", h)
			}
		}
		if out := query("SELECT count(*) FROM tpch.tiny.nation"); out != "[[25]]" {
			t.Fatalf("tpch count wrong: %s", out)
		}
		if out := query("SELECT count(*) FROM tpch.tiny.customer c JOIN tpch.tiny.nation n ON c.nationkey = n.nationkey"); out != "[[1500]]" {
			t.Fatalf("tpch join wrong: %s", out)
		}
		query("CREATE TABLE memory.default.smoke AS SELECT 42 AS x")
		if out := query("SELECT x FROM memory.default.smoke"); out != "[[42]]" {
			t.Fatalf("memory round trip wrong: %s", out)
		}
		t.Logf("smoke test passed (plugins loaded, tpch + memory queries; user %s)", u)
	})
	register("coolify-realtime", func(t *T) {
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--tmpfs", "/home/coolify:rw,mode=0700,uid=1001,gid=1001", "-p", "127.0.0.1:6001:6001", "-p", "127.0.0.1:6002:6002"})
		ok := func(url string) string {
			var body string
			t.waitFor(60, time.Second, name, url+" did not become healthy", func() bool {
				if s := t.Docker("inspect", "-f", "{{.State.Status}}", name); s != "running" {
					t.Fatalf("container exited early\n%s", t.Logs(name))
				}
				var c int
				c, body = Get(url)
				return c == 200
			})
			return body
		}
		ok("http://127.0.0.1:6001/ready")
		t.Contains(ok("http://127.0.0.1:6001/"), "OK", "soketi /")
		t.Contains(ok("http://127.0.0.1:6002/ready"), "OK", "terminal /ready")
		if c, _ := Get("http://127.0.0.1:6002/nope"); c != 404 {
			t.Fatalf("terminal GET /nope expected 404, got %d", c)
		}
		if out, err := docker("exec", "-w", "/terminal", name, "node", "-e", `import('node-pty').then(m=>{if(typeof m.spawn!=='function'){console.error('no spawn');process.exit(1)}console.log('node-pty ok')})`); err != nil {
			t.Fatalf("node-pty failed to load: %s", out)
		}
		for _, b := range []string{"ssh", "cloudflared"} {
			if _, err := docker("exec", name, "sh", "-c", "command -v "+b); err != nil {
				t.Fatalf("%s missing from runtime image", b)
			}
		}
		t.Logf("smoke test passed (nonroot uid %s, soketi :6001 + terminal :6002 healthy, node-pty, ssh, cloudflared)", t.User("1001"))
	})
	register("dotnet-runtime", func(t *T) {
		if t.Want == "" {
			t.Fatalf("usage: qw smoke dotnet-runtime <image-ref> <major>")
		}
		major := strings.SplitN(t.Want, ".", 2)[0]
		ro := []string{"--read-only", "--tmpfs", "/tmp"}
		rt := "Microsoft.NETCore.App " + major + "."
		rts, _ := t.RunFlags(ro, "--list-runtimes")
		t.Contains(rts, rt, "--list-runtimes")
		v, _ := t.RunFlags(append(ro, "--entrypoint", "/usr/bin/dotnet"), "--list-runtimes")
		t.Contains(v, rt, "/usr/bin/dotnet --list-runtimes")
		if sdks, _ := t.RunFlags(ro, "--list-sdks"); strings.Contains(sdks, major+".") {
			t.Fatalf("an SDK is installed; this image is not runtime-only: %s", sdks)
		}
		if _, err := t.RunFlags(ro, "build", "--help"); err == nil {
			t.Fatalf("'dotnet build' resolved; SDK present")
		}
		info, _ := t.RunFlags(ro, "--info")
		t.Contains(info, rt, "--info")
		t.User("1001")
		t.Contains(t.Docker("inspect", t.Image, "--format", "{{json .Config.Entrypoint}}"), "/usr/bin/dotnet", "entrypoint")
		t.Logf("smoke test passed (.NET runtime %s, runtime-only, read-only rootfs)", major)
	})
	register("fabric-peer", func(t *T) {
		v := t.RunAny("peer", "version")
		if t.Want != "" {
			t.Match(v, `(?m)Version: `+regexp.QuoteMeta(t.Want)+`$`, "peer version")
		}
		t.Match(v, `Commit SHA: [0-9a-f]{7}`, "commit stamped")
		for _, b := range []string{"detect", "build", "release"} {
			if _, err := t.RunFlags([]string{"--entrypoint", "/opt/hyperledger/ccaas_builder/bin/" + b}); err == nil {
				t.Fatalf("ccaas %s accepted no args", b)
			}
		}
		d := t.TempDir()
		cid := t.Docker("create", t.Image)
		t.Cleanup(func() { docker("rm", "-f", cid) })
		core := filepath.Join(d, "core.yaml")
		t.Docker("cp", cid+":/etc/hyperledger/fabric/core.yaml", core)
		b, _ := os.ReadFile(core)
		re := regexp.MustCompile(`(?m)^([ \t]*endpoint:) unix:///var/run/docker\.sock$`)
		if !re.Match(b) {
			t.Fatalf("vm.endpoint line not found in core.yaml")
		}
		os.WriteFile(core, re.ReplaceAll(b, []byte(`$1 ""`)), 0o644) // no docker daemon for chaincode
		name := t.Name("")
		t.Start(name, []string{"-e", "CORE_OPERATIONS_LISTENADDRESS=0.0.0.0:9443", "-v", core + ":/etc/hyperledger/fabric/core.yaml:ro", "-p", "127.0.0.1::9443"})
		ops := "http://127.0.0.1:" + t.hostPort(name, "9443/tcp")
		t.waitFor(60, time.Second, name, "peer /healthz never returned 200", func() bool {
			if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
				t.Fatalf("peer exited\n%s", t.Logs(name))
			}
			c, _ := Get(ops + "/healthz")
			return c == 200
		})
		_, ver := Get(ops + "/version")
		if t.Want != "" {
			t.Contains(ver, `"Version":"`+t.Want+`"`, "operations /version")
		}
		t.waitFor(30, time.Second, name, "peer did not report started", func() bool { return strings.Contains(t.Logs(name), "Started peer with ID") })
		t.Logf("smoke test passed (ccaas builder, healthz, version, peer started; nonroot user: %s)", t.User("1001"))
	})
}
