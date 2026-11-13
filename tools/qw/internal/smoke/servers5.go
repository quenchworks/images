package smoke

import (
	"regexp"
	"strings"
	"time"
)

// httpUp starts the image with flags and waits for url to answer 2xx; returns the body.
func (t *T) httpUp(flags []string, url string, tries int, args ...string) (string, string) {
	name := t.Name("")
	t.Start(name, flags, args...)
	return name, t.WaitHTTP(url, tries, name)
}

func init() {
	register("temporal-cli", func(t *T) {
		if t.Want == "" {
			t.Fatalf("usage: qw smoke temporal-cli <image-ref> <version>")
		}
		t.Contains(t.Run("--version"), t.Want, "version")
		t.Run("--help")
		if out, err := t.RunFlags([]string{"--read-only", "--tmpfs", "/tmp:rw"}, "env", "set", "--env", "smoke", "--key", "address", "--value", "localhost:7233"); err != nil {
			t.Fatalf("read-only rootfs + writable /tmp: %s", out)
		}
		t.Logf("smoke test passed (version %s, nonroot uid %s, read-only rootfs OK)", t.Want, t.User("1001"))
	})
	register("gitness", func(t *T) {
		u := t.User("1001")
		t.httpUp([]string{"-p", "13000:3000"}, "http://127.0.0.1:13000/", 60)
		_, v := Get("http://127.0.0.1:13000/api/v1/system/version")
		v = strings.Trim(strings.TrimSpace(v), `"`)
		t.Match(v, `^[1-9][0-9]*\.[0-9]+\.[0-9]+$`, "version stamped")
		t.Logf("smoke test passed (nonroot user: %s, UI served, version %s)", u, v)
	})
	register("emqx", func(t *T) {
		u := t.User("1001")
		name, _ := t.httpUp([]string{"-p", "18083:18083", "-p", "1883:1883"}, "http://127.0.0.1:18083/status", 120)
		out, _ := docker("exec", name, "/opt/emqx/bin/emqx_ctl", "broker")
		t.Logf("%s", out)
		t.Logf("smoke test passed (nonroot user: %s, dashboard /status healthy)", u)
	})
	register("perses", func(t *T) {
		out := t.RunEntryAny("/usr/bin/percli", "version")
		t.Match(out, `(?i)version:\s*[0-9]+\.[0-9]+\.[0-9]+`, "version stamped")
		u := t.User("1001")
		t.httpUp([]string{"-p", "18080:8080"}, "http://127.0.0.1:18080/", 40)
		t.Logf("smoke test passed (nonroot user: %s, / served)", u)
	})
	register("tetragon-operator", func(t *T) {
		t.Contains(t.RunAny("--help"), "serve", "--help lists serve")
		kc := t.DeadKubeconfig()
		out, _ := t.RunAnyFor(30*time.Second, []string{"-v", kc + ":/kc:ro"}, "serve", "--kube-config", "/kc")
		t.Match(out, `127\.0\.0\.1:1.*connection refused`, "serve reached the API call")
		t.Logf("smoke test passed (operator reached the API server, runs as %s)", t.User("1001"))
	})
	register("zitadel", func(t *T) {
		t.User("1001")
		t.Contains(t.Docker("inspect", "--format", "{{json .Config.Entrypoint}}", t.Image), "/usr/bin/zitadel", "entrypoint")
		out, err := t.RunFlags([]string{"--read-only"}, "--version")
		if err != nil {
			t.Fatalf("--version: %s", out)
		}
		t.Match(out, `(?m)^zitadel version `, "version line")
		if t.Want != "" {
			t.Match(out, `(?m)^zitadel version v?`+regexp.QuoteMeta(t.Want)+`$`, "version "+t.Want)
		}
		t.Logf("smoke test passed (%s)", out)
	})
	register("adminer", func(t *T) {
		u := t.User("1001")
		_, body := t.httpUp([]string{"--tmpfs", "/tmp", "-p", "127.0.0.1:8099:8080"}, "http://127.0.0.1:8099/", 30)
		t.ContainsFold(body, "adminer", "body mentions Adminer")
		t.ContainsFold(body, "login", "login page")
		t.Logf("smoke test passed (Adminer login served, nonroot user: %s)", u)
	})
	register("coredns", func(t *T) {
		name, h := t.httpUp([]string{"-p", "127.0.0.1:8080:8080", "-p", "127.0.0.1:8181:8181"}, "http://127.0.0.1:8080/health", 30)
		t.ContainsFold(h, "OK", "health")
		t.WaitHTTP("http://127.0.0.1:8181/ready", 10, name)
		u := t.User("1001")
		t.Logf("%s", t.Docker("exec", name, "coredns", "-version"))
		t.Logf("smoke test passed (nonroot user: %s)", u)
	})
	register("ferretdb", func(t *T) {
		v := regexp.MustCompile(`(?m)^version:.*$`).FindString(t.RunAny("--version"))
		t.Logf("%s", v)
		t.Match(v, `v[0-9]+\.[0-9]+\.[0-9]+`, "version stamped")
		if t.Want != "" {
			t.Contains(v, "v"+t.Want, "version")
		}
		t.httpUp([]string{"-p", "127.0.0.1:8088:8088"}, "http://127.0.0.1:8088/debug/livez", 30)
		t.Logf("smoke test passed (livez ok without a backend; nonroot user: %s)", t.User("1001"))
	})
	register("jaeger", func(t *T) {
		t.Match(t.RunAny("version"), `"gitVersion":"v[0-9]+\.[0-9]+\.[0-9]+"`, "version stamped")
		u := t.User("1001")
		_, body := t.httpUp([]string{"-p", "127.0.0.1:16686:16686"}, "http://127.0.0.1:16686/", 30)
		t.ContainsFold(body, "Jaeger UI", "UI")
		t.Logf("smoke test passed (nonroot user: %s, UI serving on 16686)", u)
	})
	register("mediamtx", func(t *T) {
		out := t.RunEntryAny("/usr/bin/mediamtx", "--version")
		t.Match(out, `(?i)v[0-9]+\.[0-9]+\.[0-9]+`, "version stamped")
		if strings.Contains(out, "v0.0.0") {
			t.Fatalf("version fell back to v0.0.0")
		}
		u := t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"-e", "MTX_API=yes", "-p", "19997:9997"})
		for i := 0; ; i++ {
			if c, _ := Get("http://127.0.0.1:19997/v3/config/global/get"); c != 0 {
				t.Logf("smoke test passed (nonroot user: %s, API on 9997 responded HTTP %d)", u, c)
				return
			}
			if i == 20 {
				t.Fatalf("API on 9997 did not respond\n%s", t.Logs(name))
			}
			time.Sleep(500 * time.Millisecond)
		}
	})
	register("nats", func(t *T) {
		name, h := t.httpUp([]string{"-p", "127.0.0.1:8222:8222"}, "http://127.0.0.1:8222/healthz", 30, "-js", "-sd", "/data")
		t.Contains(h, `"status":"ok"`, "healthz")
		_, varz := Get("http://127.0.0.1:8222/varz")
		t.Contains(varz, `"jetstream"`, "varz reports JetStream")
		u := t.User("1001")
		t.Logf("%s", t.Docker("exec", name, "nats-server", "--version"))
		t.Logf("smoke test passed (nonroot user: %s)", u)
	})
}
