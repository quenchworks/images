package smoke

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func init() {
	register("fabric-orderer", func(t *T) {
		v := t.RunAny("orderer", "version")
		if t.Want != "" {
			t.Match(v, `(?m)Version: `+regexp.QuoteMeta(t.Want)+`$`, "orderer version")
		}
		t.Match(v, `Commit SHA: [0-9a-f]{7}`, "commit stamped")
		major := regexp.MustCompile(`(?m)^ *Version: ([0-9]+)\.`).FindStringSubmatch(v)
		const m = "/etc/hyperledger/fabric/msp"
		flags := []string{"-e", "ORDERER_GENERAL_TLS_ENABLED=true", "-e", "ORDERER_GENERAL_TLS_PRIVATEKEY=" + m + "/keystore/key.pem",
			"-e", "ORDERER_GENERAL_TLS_CERTIFICATE=" + m + "/signcerts/peer.pem", "-e", "ORDERER_GENERAL_TLS_ROOTCAS=[" + m + "/cacerts/cacert.pem]",
			"-e", "ORDERER_OPERATIONS_LISTENADDRESS=0.0.0.0:8443", "-e", "ORDERER_ADMIN_LISTENADDRESS=0.0.0.0:9443", "-p", "127.0.0.1::8443", "-p", "127.0.0.1::9443"}
		if major != nil && major[1] == "2" {
			flags = append(flags, "-e", "ORDERER_GENERAL_BOOTSTRAPMETHOD=none", "-e", "ORDERER_CHANNELPARTICIPATION_ENABLED=true")
		}
		name := t.Name("")
		t.Start(name, flags)
		ops, adm := "http://127.0.0.1:"+t.hostPort(name, "8443/tcp"), "http://127.0.0.1:"+t.hostPort(name, "9443/tcp")
		t.waitFor(60, time.Second, name, "orderer /healthz never returned 200", func() bool {
			if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
				t.Fatalf("orderer exited\n%s", t.Logs(name))
			}
			c, _ := Get(ops + "/healthz")
			return c == 200
		})
		_, ch := Get(adm + "/participation/v1/channels")
		t.Contains(ch, `"channels"`, "channel participation API")
		_, ver := Get(ops + "/version")
		if t.Want != "" {
			t.Contains(ver, `"Version":"`+t.Want+`"`, "operations /version")
		}
		t.Logf("smoke test passed (healthz, admin channels, version; nonroot user: %s)", t.User("1001"))
	})
	register("ruby", func(t *T) {
		if t.Want == "" {
			t.Fatalf("usage: qw smoke ruby <image-ref> <minor>")
		}
		minor := strings.Join(strings.SplitN(t.Want, ".", 3)[:2], ".")
		ro := []string{"--read-only", "--tmpfs", "/tmp"}
		if v, _ := t.RunFlags(ro, "-v"); !strings.HasPrefix(v, "ruby "+minor+".") {
			t.Fatalf("expected 'ruby %s.*', got '%s'", minor, v)
		}
		if out, _ := t.RunFlags(ro, "-e", `print "quench-" + "ruby"`); out != "quench-ruby" {
			t.Fatalf("script output wrong: '%s'", out)
		}
		std, _ := t.RunFlags(ro, "-e", `require "openssl"; require "json"; require "digest"; puts "stdlib ok " + OpenSSL::OPENSSL_VERSION`)
		t.Contains(std, "stdlib ok", "stdlib")
		gv, _ := t.RunFlags(append(ro, "--entrypoint", "/usr/bin/gem"), "--version")
		t.Match(gv, `^[0-9]`, "gem --version")
		bv, _ := t.RunFlags(append(ro, "--entrypoint", "/usr/bin/bundle"), "--version")
		t.Match(bv, `^(Bundler version )?[0-9]`, "bundler")
		if uid, _ := t.RunFlags(ro, "-e", "print Process.uid"); uid != "1001" {
			t.Fatalf("expected uid 1001, got '%s'", uid)
		}
		t.User("1001")
		if _, err := t.RunFlags(ro, "-e", `File.write("/should-fail","x")`); err == nil {
			t.Fatalf("rootfs was writable, expected read-only")
		}
		t.Logf("smoke test passed (Ruby %s, gem %s, read-only rootfs)", minor, gv)
	})
	register("composer", func(t *T) {
		if t.Want == "" {
			t.Fatalf("usage: qw smoke composer <image-ref> <phpminor>")
		}
		ro := []string{"--read-only", "--tmpfs", "/tmp"}
		v, _ := t.RunFlags(ro, "--version")
		t.Contains(v, "Composer version 2.", "composer 2.x")
		t.Contains(v, "PHP version "+t.Want+".", "composer's PHP")
		pv, _ := t.RunFlags(append(ro, "--entrypoint", "/usr/bin/php"), "-v")
		if !strings.HasPrefix(pv, "PHP "+t.Want+".") {
			t.Fatalf("expected 'PHP %s.*', got '%s'", t.Want, pv)
		}
		gv, _ := t.RunFlags(append(ro, "--entrypoint", "/usr/bin/git"), "--version")
		t.Match(gv, `^git version `, "git present")
		if out, err := t.RunFlags(ro, "config", "--global", "--list"); err != nil {
			t.Fatalf("composer could not use its global config under /tmp: %s", out)
		}
		if uid, _ := t.RunFlags(append(ro, "--entrypoint", "/usr/bin/php"), "-r", "echo getmyuid();"); uid != "1001" {
			t.Fatalf("expected uid 1001, got '%s'", uid)
		}
		t.User("1001")
		if _, err := t.RunFlags(append(ro, "--entrypoint", "/usr/bin/php"), "-r", `exit(@file_put_contents("/should-fail","x") === false ? 0 : 1);`); err != nil {
			t.Fatalf("rootfs was writable, expected read-only")
		}
		t.Logf("smoke test passed (%s, read-only rootfs)", strings.SplitN(v, "\n", 2)[0])
	})
	register("grafana-alloy", func(t *T) {
		t.Match(t.RunAny("--version"), `(?i)alloy, version v?[0-9]+\.[0-9]+\.[0-9]+`, "version stamped")
		w := t.TempDir()
		os.WriteFile(filepath.Join(w, "config.alloy"), []byte("logging {\n  level  = \"info\"\n  format = \"logfmt\"\n}\n"), 0o644)
		t.httpUp([]string{"--read-only", "--tmpfs", "/tmp:rw,uid=1001,gid=1001", "--tmpfs", "/var/lib/alloy/data:rw,uid=1001,gid=1001",
			"-v", w + "/config.alloy:/etc/alloy/config.alloy:ro", "-p", "127.0.0.1:12345:12345"}, "http://127.0.0.1:12345/-/ready", 60,
			"run", "/etc/alloy/config.alloy", "--server.http.listen-addr=0.0.0.0:12345", "--storage.path=/var/lib/alloy/data")
		t.Logf("smoke test passed (nonroot user: %s, read-only rootfs, /-/ready)", t.User("1001"))
	})
	register("grist", func(t *T) {
		const key = "quench-smoke-boot-key-0123456789"
		u := t.User("1001")
		v := t.Run("-p", `require("/grist/package.json").version`)
		if t.Want != "" && v != t.Want {
			t.Fatalf("grist %s, expected %s", v, t.Want)
		}
		vol, name := t.Name("data"), t.Name("")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,exec", "-v", vol + ":/persist", "--cap-drop", "ALL", "--security-opt", "no-new-privileges",
			"-p", "127.0.0.1:18484:8484", "-e", "GRIST_BOOT_KEY=" + key, "-e", "GRIST_DEFAULT_EMAIL=admin@example.com"})
		t.WaitHTTP("http://127.0.0.1:18484/status", 90, name)
		const b = "http://127.0.0.1:18484/o/docs/api"
		h := map[string]string{"x-boot-key": key, "Content-Type": "application/json"}
		api := func(method, path, body string) string {
			c, r := DoH(method, b+path, body, h)
			if c/100 != 2 {
				t.Fatalf("%s %s: %d %s\n%s", method, path, c, r, t.Logs(name))
			}
			return r
		}
		var ws []struct{ ID int }
		json.Unmarshal([]byte(api("GET", "/orgs/current/workspaces", "")), &ws)
		if len(ws) == 0 {
			t.Fatalf("no workspace id")
		}
		doc := strings.Trim(api("POST", "/workspaces/"+itoa(ws[0].ID)+"/docs", `{"name":"quench-smoke"}`), `"`)
		api("POST", "/docs/"+doc+"/tables", `{"tables":[{"id":"Smoke","columns":[{"id":"A","fields":{"type":"Numeric"}},{"id":"B","fields":{"type":"Numeric","isFormula":true,"formula":"$A * 6 + len(\"abc\")"}},{"id":"H","fields":{"type":"Text","isFormula":true,"formula":"__import__(\"os\").uname()[1]"}}]}]}`)
		api("POST", "/docs/"+doc+"/tables/Smoke/records", `{"records":[{"fields":{"A":7}}]}`)
		out := api("GET", "/docs/"+doc+"/tables/Smoke/records", "")
		t.Contains(out, `"B":45`, "formula computed 45")
		t.Contains(out, `"H":"gristland"`, "formula ran inside the runsc sandbox")
		t.Contains(t.Logs(name), "flavor=gvisor", "gvisor sandbox flavor")
		t.Logf("smoke test passed (gVisor sandbox computed the formula; user %s)", u)
	})
}
func itoa(n int) string { return strconv.Itoa(n) }
