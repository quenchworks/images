package smoke

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

func init() {
	// DB-less on a read-only rootfs: the Admin API answers with the version, the OpenResty
	// configure line carries Kong's own nginx modules, atc-router is present (without it no
	// route ever matches), and a declarative config proxies a request through to an upstream.
	register("kong", func(t *T) {
		const proxy, admin = "http://127.0.0.1:18000", "http://127.0.0.1:18001"
		u := t.User("1001")
		net_, up, name := t.Name("net"), t.Name("upstream"), t.Name("")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Cleanup(func() { docker("rm", "-f", up) })
		t.Docker("run", "-d", "--name", up, "--network", net_, envOr("UPSTREAM_IMAGE", "nginx:alpine"))
		t.waitFor(20, time.Second, up, "test upstream never served", func() bool {
			_, err := docker("exec", up, "sh", "-c", "wget -qO- http://127.0.0.1/")
			return err == nil
		})
		t.Start(name, []string{"--network", net_, "--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--tmpfs", "/kong_prefix:rw,mode=0755,uid=1001,gid=1001",
			"-p", "127.0.0.1:18000:8000", "-p", "127.0.0.1:18001:8001"})
		var info string
		t.waitFor(60, time.Second, name, "Admin API never returned 200", func() bool {
			c, b := Get(admin + "/")
			info = b
			return c == 200
		})
		var root struct {
			Version string `json:"version"`
		}
		if json.Unmarshal([]byte(info), &root) != nil || root.Version == "" {
			t.Fatalf("Admin API returned no version: %.300s", info)
		}
		if t.Want != "" && !strings.HasPrefix(root.Version, t.Want) {
			t.Fatalf("kong %s, expected %s", root.Version, t.Want)
		}
		conf, _ := docker("exec", name, "/usr/local/openresty/nginx/sbin/nginx", "-V")
		for _, m := range []string{"lua-kong-nginx-module", "lua-resty-lmdb", "lua-resty-events"} {
			t.Contains(conf, m, "nginx configure line")
		}
		t.Docker("exec", name, "test", "-f", "/usr/local/lib/lua/5.1/libatc_router.so")

		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		fw, _ := mw.CreateFormFile("config", "kong.yml")
		io.WriteString(fw, "_format_version: \"3.0\"\nservices:\n  - name: echo\n    url: http://"+up+":80\n    routes:\n      - name: echo-route\n        paths: [/echo]\n        strip_path: true\n")
		mw.Close()
		resp, err := http.Post(admin+"/config", mw.FormDataContentType(), &body)
		if err != nil {
			t.Fatalf("POST /config: %v", err)
		}
		rb, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if !strings.Contains(string(rb), `"services"`) {
			t.Fatalf("declarative config rejected (%d): %.600s", resp.StatusCode, rb)
		}
		t.waitFor(20, time.Second, name, "request did not traverse the proxy to the upstream", func() bool {
			_, b := Get(proxy + "/echo")
			return strings.Contains(b, "Welcome to nginx")
		})
		t.Logf("smoke test passed (kong %s: admin API, Kong nginx modules, atc-router, declarative proxy round trip; user %s)", root.Version, u)
	})
}
