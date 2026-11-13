package smoke

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// nodeTool checks a Node-based CLI image's runtime: node -v, uid 1001, read-only root.
func nodeTool(t *T) string {
	ro := []string{"--read-only", "--tmpfs", "/tmp", "--entrypoint", "/usr/bin/node"}
	nv, _ := t.RunFlags(ro, "-v")
	t.Match(nv, `^v[0-9]`, "node -v")
	if uid, _ := t.RunFlags(ro, "-e", "process.stdout.write(String(process.getuid()))"); uid != "1001" {
		t.Fatalf("expected uid 1001, got '%s'", uid)
	}
	t.User("1001")
	if _, err := t.RunFlags(ro, "-e", `require("fs").writeFileSync("/should-fail","x")`); err == nil {
		t.Fatalf("rootfs was writable, expected read-only")
	}
	return nv
}

func init() {
	register("dapr", func(t *T) {
		v := lastLine(t.RunAny("--version"))
		if v == "" || regexp.MustCompile(`edge|dev`).MatchString(v) || (t.Want != "" && v != t.Want) {
			t.Fatalf("daprd version '%s', expected %s", v, t.Want)
		}
		bins := []string{"operator", "placement", "sentry", "injector", "scheduler"}
		for _, b := range bins {
			for _, p := range []string{"/usr/bin/" + b, "/" + b} {
				t.Match(t.RunEntryAny(p, "-h"), `(?i)usage|flags`, p+" starts")
			}
		}
		if lv := lastLine(t.RunEntryAny("/daprd", "--version")); lv != v {
			t.Fatalf("/daprd link missing (%s)", lv)
		}
		name := t.Name("")
		t.Start(name, []string{"-p", "127.0.0.1:13500:3500", "--read-only", "--tmpfs", "/tmp"}, "--app-id", "quench-smoke", "--mode", "standalone",
			"--dapr-http-port", "3500", "--resources-path", "/tmp", "--dapr-listen-addresses", "0.0.0.0", "--log-level", "info")
		t.waitCode("http://127.0.0.1:13500/v1.0/healthz/outbound", 40, time.Second, name, 204)
		_, meta := Get("http://127.0.0.1:13500/v1.0/metadata")
		t.Contains(meta, `"id":"quench-smoke"`, "metadata app id")
		t.Logf("smoke test passed (Dapr %s, nonroot user: %s, control plane starts, daprd self-hosted)", v, t.User("1001"))
	})
	register("floci-full", func(t *T) {
		u := t.rootLikeUpstream()
		t.RunEntryAny("/usr/lib/jvm/java-25-openjdk/bin/java", "-version")
		name := t.Name("")
		t.Start(name, []string{"-p", "127.0.0.1:0:4566"})
		base := "http://127.0.0.1:" + t.hostPort(name, "4566/tcp")
		t.waitCode(base+"/_floci/health", 60, 2*time.Second, name, 200)
		_, body := Get(base + "/_floci/health")
		t.Contains(body, `"services"`, "health services map")
		t.Logf("smoke test passed (root user %q, health with services map)", u)
	})
	register("gitleaks", func(t *T) {
		u := t.User("1001")
		if v := t.RunAny("version"); t.Want != "" {
			t.Contains(v, t.Want, "version")
		}
		w := t.TempDir()
		leaky, clean := filepath.Join(w, "leaky"), filepath.Join(w, "clean")
		for _, d := range []string{leaky, clean} {
			os.Mkdir(d, 0o777)
			os.Chmod(d, 0o777)
		}
		os.WriteFile(filepath.Join(leaky, "config.ini"), []byte("aws_access_key_id = AKIA"+"QYLPMN5HHHFPZAM2\naws_secret_access_key = wJalrXUtnFEMI/K7MDENG/"+"bPxRfiCYzEXAMPLEKEY1\n"), 0o666)
		os.WriteFile(filepath.Join(clean, "config.ini"), []byte("hello = \"world\"\n"), 0o644)
		if out, code := t.RunCode([]string{"--read-only", "-v", leaky + ":/scan:ro"}, "dir", "/scan", "--no-banner"); code != 1 {
			t.Fatalf("planted key not reported (exit %d): %s", code, out)
		}
		if out, code := t.RunCode([]string{"--read-only", "-v", clean + ":/scan:ro"}, "dir", "/scan", "--no-banner"); code != 0 {
			t.Fatalf("clean directory reported (exit %d): %s", code, out)
		}
		// commit the key, then delete it: only history holds it now (git from the image itself)
		t.leakHistory(leaky)
		if out, code := t.RunCode([]string{"--read-only", "--tmpfs", "/tmp", "-e", "GIT_CONFIG_COUNT=1", "-e", "GIT_CONFIG_KEY_0=safe.directory", "-e", "GIT_CONFIG_VALUE_0=*",
			"-v", leaky + ":/repo:ro"}, "git", "/repo", "--no-banner"); code != 1 {
			t.Fatalf("key in history not reported (exit %d): %s", code, out)
		}
		t.Logf("smoke test passed (dir and git history scans, uid %s)", u)
	})
	register("kgateway", func(t *T) {
		var v struct{ Version string }
		json.Unmarshal([]byte(t.Run("--version")), &v)
		if v.Version == "" || v.Version == "undefined" || v.Version == "v0.0.0" || strings.Contains(v.Version, "dev") {
			t.Fatalf("version not stamped: '%s'", v.Version)
		}
		if t.Want != "" && v.Version != "v"+t.Want {
			t.Fatalf("expected v%s, got %s", t.Want, v.Version)
		}
		if out, code := t.RunAnyFor(120*time.Second, nil); code == 0 || code == 124 {
			t.Fatalf("controller should fail fast without an API server (exit %d):\n%s", code, out)
		}
		t.RunEntry("/usr/local/bin/envoy", "--version")
		ep := []string{"--entrypoint", "/usr/local/bin/envoy"}
		val := []string{"--mode", "validate", "--config-path", "/dev/fd/0", "-l", "critical"}
		if out, err := t.RunInputFlags(ep, `{"admin":{"address":{"socket_address":{"address":"127.0.0.1","port_value":19000}}}}`, val...); err != nil {
			t.Fatalf("envoy rejected a valid bootstrap: %s", out)
		}
		if _, err := t.RunInputFlags(ep, `{"admin":{"not_a_field":1}}`, val...); err == nil {
			t.Fatalf("envoy accepted an invalid bootstrap; validation is not running")
		}
		t.Logf("smoke test passed (kgateway %s, user %s)", v.Version, t.Docker("inspect", t.Image, "--format", "{{.Config.User}}"))
	})
	register("label-studio", func(t *T) {
		const tok, b = "quenchsmoketoken1234", "http://127.0.0.1:18080"
		u := t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"-p", "127.0.0.1:18080:8080", "-e", "LABEL_STUDIO_ENABLE_LEGACY_API_TOKEN=true", "-e", "LABEL_STUDIO_USERNAME=admin@example.com",
			"-e", "LABEL_STUDIO_PASSWORD=quench-smoke-pass", "-e", "LABEL_STUDIO_USER_TOKEN=" + tok})
		t.waitFor(120, 2*time.Second, name, "/health never came UP", func() bool { _, h := Get(b + "/health"); return strings.Contains(h, `"UP"`) })
		var ver struct{ Release string }
		_, vb := Get(b + "/api/version")
		if json.Unmarshal([]byte(vb), &ver) != nil || ver.Release == "" || (t.Want != "" && ver.Release != t.Want) {
			t.Fatalf("release %q, expected %s", ver.Release, t.Want)
		}
		auth := map[string]string{"Authorization": "Token " + tok, "Content-Type": "application/json"}
		if c, r := DoH("POST", b+"/api/projects", `{"title":"smoke","label_config":"<View><Text name=\"t\" value=\"$text\"/><Choices name=\"c\" toName=\"t\"><Choice value=\"a\"/></Choices></View>"}`, auth); c/100 != 2 {
			t.Fatalf("project create: %d %s", c, r)
		}
		_, ps := DoH("GET", b+"/api/projects", "", auth)
		t.Match(ps, `"title": *"smoke"`, "project listed")
		if c, _ := DoH("GET", b+"/api/projects", "", map[string]string{"Authorization": "Token wrong-token"}); c != 401 {
			t.Fatalf("bad token got HTTP %d, want 401", c)
		}
		t.Logf("smoke test passed (label-studio %s: health, version, project API, token auth; nonroot user: %s)", ver.Release, u)
	})
	register("litellm", func(t *T) {
		if t.Want == "" {
			t.Fatalf("usage: qw smoke litellm <image-ref> <version>")
		}
		u := t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"-p", "127.0.0.1:14000:4000"})
		refused := false
		t.waitFor(90, time.Second, name, "proxy never answered /health/liveliness", func() bool {
			if c, _ := Get("http://127.0.0.1:14000/health/liveliness"); c == 200 {
				return true
			}
			if t.Docker("inspect", "-f", "{{.State.Running}}", name) == "false" {
				t.Contains(t.Logs(name), "UnsafeMasterKeyError", "exited by refusing to run without a master key")
				refused = true
				return true
			}
			return false
		})
		if !refused {
			if c, _ := Get("http://127.0.0.1:14000/v1/models"); c/100 != 2 {
				t.Fatalf("/v1/models did not answer (%d)", c)
			}
		}
		docker("rm", "-f", name)
		t.Start(name, []string{"-e", "LITELLM_MASTER_KEY=sk-smoke", "-p", "127.0.0.1:14000:4000"})
		t.WaitHTTP("http://127.0.0.1:14000/health/liveliness", 90, name)
		if c, _ := Get("http://127.0.0.1:14000/v1/models"); c != 401 {
			t.Fatalf("expected 401 without the key, got %d", c)
		}
		if c, _ := DoH("GET", "http://127.0.0.1:14000/v1/models", "", map[string]string{"Authorization": "Bearer sk-smoke"}); c != 200 {
			t.Fatalf("/v1/models with the key: %d", c)
		}
		if v := t.Docker("exec", name, "/opt/litellm/venv/bin/python", "-c", "import importlib.metadata as m; print(m.version('litellm'))"); v != t.Want {
			t.Fatalf("litellm %s, expected %s", v, t.Want)
		}
		if _, err := docker("exec", name, "/opt/litellm/venv/bin/python", "-c", "import litellm_enterprise"); err == nil {
			t.Fatalf("litellm_enterprise is importable; the image must stay MIT")
		}
		t.Logf("smoke test passed (nonroot %s, litellm %s, no enterprise package)", u, t.Want)
	})
	register("node", func(t *T) {
		if t.Want == "" {
			t.Fatalf("usage: qw smoke node <image-ref> <expect>")
		}
		ro := []string{"--read-only", "--tmpfs", "/tmp"}
		v, _ := t.RunFlags(ro, "-v")
		if v != "v"+t.Want && !strings.HasPrefix(v, "v"+t.Want+".") {
			t.Fatalf("expected 'v%s', got '%s'", t.Want, v)
		}
		if out, _ := t.RunFlags(ro, "-e", `process.stdout.write("quench-" + "node")`); out != "quench-node" {
			t.Fatalf("script output wrong: '%s'", out)
		}
		t.Contains(t.Run("-e", `const c=require("crypto"),t=require("tls"); console.log("core ok", c.createHash("sha256").update("x").digest("hex").slice(0,8))`), "core ok", "crypto/tls")
		npm, _ := t.RunFlags(append(ro, "--entrypoint", "/usr/bin/node"), "/usr/bin/npm", "-v")
		t.Match(npm, `^[0-9]`, "npm -v")
		nodeTool(t)
		t.Logf("smoke test passed (Node.js %s, npm %s, read-only rootfs)", v, npm)
	})
	register("pnpm", func(t *T) {
		if t.Want == "" {
			t.Fatalf("usage: qw smoke pnpm <image-ref> <version>")
		}
		ro := []string{"--read-only", "--tmpfs", "/tmp"}
		v, _ := t.RunFlags(ro, "--version")
		if v != t.Want && !strings.HasPrefix(v, t.Want+".") {
			t.Fatalf("expected '%s', got '%s'", t.Want, v)
		}
		sp, _ := t.RunFlags(append(ro, "-w", "/tmp"), "store", "path")
		if !strings.HasPrefix(sp, "/tmp/") {
			t.Fatalf("expected pnpm store under /tmp, got '%s'", sp)
		}
		nv := nodeTool(t)
		t.Logf("smoke test passed (pnpm %s, node %s, read-only rootfs)", v, nv)
	})
}
