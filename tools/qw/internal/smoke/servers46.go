package smoke

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func init() {
	register("openresty", func(t *T) {
		t.User("1001")
		ver := t.Match(t.RunAny("-v"), `openresty/([0-9.]*[0-9])`, "openresty version from -v")
		t.Logf("openresty version: %s", ver)
		if t.Want != "" && ver != t.Want {
			t.Fatalf("expected %s", t.Want)
		}
		// lua-nginx-module, resty.core and the libcrypto FFI linkage, not just a port.
		dir := t.TempDir()
		conf := filepath.Join(dir, "lua.conf")
		os.WriteFile(conf, []byte(`server {
    listen 8081;
    location = /lua {
        default_type text/plain;
        content_by_lua_block { ngx.say("lua ok ", jit.version) }
    }
    location = /sha {
        default_type text/plain;
        content_by_lua_block {
            local sha = require("resty.sha256"):new()
            sha:update("quench")
            ngx.say(require("resty.string").to_hex(sha:final()))
        }
    }
}
`), 0o644)
		name, page := t.httpUp([]string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", conf + ":/etc/nginx/conf.d/lua.conf:ro",
			"-p", "127.0.0.1:8080:8080", "-p", "127.0.0.1:8081:8081"}, "http://127.0.0.1:8080/", 30)
		t.ContainsFold(page, "openresty", "default page")
		_, st := Get("http://127.0.0.1:8080/stub_status")
		t.Contains(st, "Active connections", "stub_status")
		_, lua := Get("http://127.0.0.1:8081/lua")
		t.Logf("%s", strings.TrimSpace(lua))
		t.Match(lua, `^lua ok LuaJIT 2\.1`, "Lua handler")
		if _, sha := Get("http://127.0.0.1:8081/sha"); strings.TrimSpace(sha) != "a8b51e95fe15708a5f253f567e72f00f052cd6c11f013b19c5b122bc52b98073" {
			t.Fatalf("resty.sha256 returned %q\n%s", sha, t.Logs(name))
		}
		if m := regexp.MustCompile(`(?i)read-only file system|permission denied|\[emerg\]|\[alert\]`).FindString(t.Logs(name)); m != "" {
			t.Fatalf("errors in the log (%s)\n%s", m, t.Logs(name))
		}
		t.Logf("smoke test passed (openresty %s, LuaJIT + resty.sha256 ok, nonroot user: 1001)", ver)
	})
	register("apache-polaris", func(t *T) {
		t.User("1001")
		// FILE storage is insecure by design: refused unless allowed, flagged by readiness.
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "application.properties"), []byte(`polaris.features."SUPPORTED_CATALOG_STORAGE_TYPES"=["FILE"]
polaris.features."ALLOW_INSECURE_STORAGE_TYPES"=true
polaris.readiness.ignore-severe-issues=true
`), 0o644)
		os.Chmod(dir, 0o755)
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp", "-e", "POLARIS_BOOTSTRAP_CREDENTIALS=POLARIS,root,s3cr3t",
			"-v", dir + ":/home/nonroot/config:ro", "-p", "127.0.0.1:18181:8181", "-p", "127.0.0.1:18182:8182"})
		t.waitFor(120, time.Second, name, "polaris never became ready", func() bool {
			_, h := Get("http://127.0.0.1:18182/q/health/ready")
			return regexp.MustCompile(`"status": *"UP"`).MatchString(h)
		})
		const b = "http://127.0.0.1:18181/api"
		_, tok := Post(b+"/catalog/v1/oauth/tokens", "application/x-www-form-urlencoded",
			"grant_type=client_credentials&client_id=root&client_secret=s3cr3t&scope=PRINCIPAL_ROLE:ALL")
		var tr struct {
			AccessToken string `json:"access_token"`
		}
		if json.Unmarshal([]byte(tok), &tr); tr.AccessToken == "" {
			t.Fatalf("no token: %s", tok)
		}
		h := map[string]string{"Authorization": "Bearer " + tr.AccessToken, "Content-Type": "application/json"}
		call := func(method, path, body string) string {
			c, out := DoH(method, b+path, body, h)
			if c/100 != 2 {
				t.Fatalf("%s %s: HTTP %d %s", method, path, c, out)
			}
			return out
		}
		call("POST", "/management/v1/catalogs", `{"catalog":{"name":"quench","type":"INTERNAL","properties":{"default-base-location":"file:///tmp/wh"},"storageConfigInfo":{"storageType":"FILE","allowedLocations":["file:///tmp/wh"]}}}`)
		t.Match(call("GET", "/management/v1/catalogs", ""), `"name" *: *"quench"`, "catalog listed")
		call("POST", "/catalog/v1/quench/namespaces", `{"namespace":["smoke"]}`)
		t.Contains(call("GET", "/catalog/v1/quench/namespaces", ""), `"smoke"`, "namespace listed")
		call("POST", "/catalog/v1/quench/namespaces/smoke/tables", `{"name":"t1","schema":{"type":"struct","fields":[{"id":1,"name":"id","type":"long","required":true}]}}`)
		t.Match(call("GET", "/catalog/v1/quench/namespaces/smoke/tables", ""), `"name" *: *"t1"`, "table listed")
		if meta, _ := docker("exec", name, "find", "/tmp/wh", "-name", "*.metadata.json"); strings.TrimSpace(meta) == "" {
			t.Fatalf("no table metadata written under /tmp/wh")
		}
		logs := t.Logs(name)
		t.WantVersion(logs)
		// FILE storage logs a severe readiness ERROR by design; fail on class loading only.
		if m := regexp.MustCompile(`ClassNotFound|NoClassDefFound|LinkageError`).FindString(logs); m != "" {
			t.Fatalf("polaris hit a class loading error (%s)", m)
		}
		adm, _ := docker("run", "--rm", "--read-only", "--tmpfs", "/tmp", "--entrypoint", "/usr/lib/jvm/java-21-openjdk/bin/java",
			t.Image, "-jar", "/usr/lib/polaris/admin/quarkus-run.jar", "--help")
		t.Contains(adm, "bootstrap", "admin tool --help")
		t.Logf("smoke test passed (polaris %s, uid 1001, catalog, namespace and table created and listed)", t.Want)
	})
	register("frp", func(t *T) {
		t.User("1001")
		for _, b := range []string{"frps", "frpc"} {
			v := strings.TrimSpace(t.RunEntry("/usr/bin/"+b, "--version"))
			t.Logf("%s %s", b, v)
			if t.Want != "" && v != t.Want {
				t.Fatalf("%s: expected %s", b, t.Want)
			}
		}
		id := t.Name("")
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "frps.toml"), []byte(`bindPort = 7000
auth.token = "smoke-token"
webServer.addr = "0.0.0.0"
webServer.port = 7500
webServer.user = "admin"
webServer.password = "smoke-pass"
`), 0o644)
		// The client exposes the server's own dashboard back through remote port 6000.
		os.WriteFile(filepath.Join(dir, "frpc.toml"), []byte(`serverAddr = "`+id+`-s"
serverPort = 7000
auth.token = "smoke-token"
[[proxies]]
name = "dash"
type = "tcp"
localIP = "`+id+`-s"
localPort = 7500
remotePort = 6000
`), 0o644)
		t.Docker("network", "create", id)
		t.Cleanup(func() { docker("network", "rm", id) })
		t.Start(id+"-s", []string{"--network", id, "--read-only", "-v", dir + ":/conf:ro", "-p", "127.0.0.1:17500:7500", "-p", "127.0.0.1:16000:6000"}, "-c", "/conf/frps.toml")
		t.Start(id+"-c", []string{"--network", id, "--read-only", "-v", dir + ":/conf:ro", "--entrypoint", "/usr/bin/frpc"}, "-c", "/conf/frpc.toml")
		auth := func(pw string) map[string]string {
			return map[string]string{"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:"+pw))}
		}
		t.waitFor(30, time.Second, id+"-c", "dashboard not reachable through the tunnel", func() bool {
			c, page := DoH("GET", "http://127.0.0.1:16000/", "", auth("smoke-pass"))
			return c == 200 && strings.Contains(strings.ToLower(page), "<html")
		})
		t.Logf("tunnel ok: frps dashboard served through frpc on remote port 6000")
		_, info := DoH("GET", "http://127.0.0.1:17500/api/serverinfo", "", auth("smoke-pass"))
		t.Contains(info, `"version"`, "serverinfo")
		if c, _ := DoH("GET", "http://127.0.0.1:17500/api/serverinfo", "", auth("wrong")); c != 401 {
			t.Fatalf("wrong password got HTTP %d", c)
		}
		t.Logf("smoke test passed: tunnel, dashboard API, auth refusal, user 1001")
	})
}
