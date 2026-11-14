package smoke

import (
	"os"
	"regexp"
	"strings"
	"time"
)

func init() {
	// Beyond index/get: the SQL plugin (the patched calcite planner), a remote reindex (the REST
	// client and the standalone httpclient5, where graylog broke on gzip), and a webhook
	// notification (the httpclient5 relocated into notifications-core-spi: a bad relocation gives a
	// linkage error instead of the sink's HTTP reason phrase).
	register("opensearch", func(t *T) {
		const base = "http://127.0.0.1:19200"
		u := t.User("1001")
		name := t.Name("")
		// OpenSearch 3.x reads reindex.remote.allowlist from opensearch.yml only; the entrypoint's
		// OPENSEARCH_CONFIG_EXTRA appends raw yml
		t.Start(name, []string{"-p", "127.0.0.1:19200:9200", "-e", `OPENSEARCH_CONFIG_EXTRA=reindex.remote.allowlist: ["127.0.0.1:9200", "localhost:9200"]`})
		t.waitFor(60, 3*time.Second, name, "opensearch did not become ready", func() bool {
			_, b := Get(base + "/")
			return strings.Contains(b, `"cluster_name"`)
		})
		_, h := Get(base + "/_cluster/health")
		t.Match(h, `"status":"(green|yellow)"`, "cluster health")
		j := map[string]string{"Content-Type": "application/json"}
		DoH("POST", base+"/smoke/_doc/1?refresh=true", `{"k":"quench"}`, j)
		_, doc := Get(base + "/smoke/_doc/1")
		t.Contains(doc, `"quench"`, "index and get")
		_, sql := DoH("POST", base+"/_plugins/_sql", `{"query": "SELECT k FROM smoke"}`, j)
		t.Contains(sql, "quench", "SQL plugin SELECT (the calcite planner)")
		_, rr := DoH("POST", base+"/_reindex?refresh=true", `{"source": {"remote": {"host": "http://127.0.0.1:9200"}, "index": "smoke"}, "dest": {"index": "smoke-remote"}}`, j)
		if !strings.Contains(rr, `"failures":[]`) {
			t.Fatalf("remote reindex failed (httpclient5/gzip first suspect): %s\n%s", rr, t.Logs(name))
		}
		_, rdoc := Get(base + "/smoke-remote/_doc/1")
		t.Contains(rdoc, `"quench"`, "remote-reindexed document")
		DoH("POST", base+"/_plugins/_notifications/configs", `{"config_id": "smoke-webhook", "config": {"name": "smoke", "description": "smoke test webhook",
			"config_type": "webhook", "is_enabled": true, "webhook": {"url": "http://localhost:9200/hooksink/_doc/1"}}}`, j)
		// the sink (this node) refuses the plain-text test message: 400 on 3.8, 406 on 3.9; either
		// reason phrase coming back through the shaded client is the assertion
		_, wh := DoH("POST", base+"/_plugins/_notifications/feature/test/smoke-webhook", "", nil)
		if regexp.MustCompile(`NoClassDefFoundError|ClassNotFoundException|VerifyError|NoSuchMethodError`).MatchString(wh) {
			t.Fatalf("the shaded httpclient5 failed to link: %s", wh)
		}
		t.Match(wh, `Failed: (Bad Request|Not Acceptable)`, "webhook answer through the shaded httpclient5")
		t.Logf("smoke test passed (opensearch %s: health, index, SQL, remote reindex, webhook through the shaded client; user %s)", t.Want, u)
	})

	// scanner-trivy boots on a read-only rootfs and answers its liveness probe without Redis; the
	// bundled trivy CLI is the version build.conf's TRIVY map pairs with this release.
	register("harbor-trivy-adapter", func(t *T) {
		u := t.User("1001")
		if t.Want == "" {
			t.Fatalf("usage: qw smoke harbor-trivy-adapter <image-ref> <app-version>")
		}
		// the TRIVY map only: a SCANNER map with the same keys sits on another line
		bc, _ := os.ReadFile("../../apps/harbor-trivy-adapter/build.conf")
		m := regexp.MustCompile(`(?m)^declare -A TRIVY=.*\[` + regexp.QuoteMeta(t.Want) + `\]=([0-9.]+)`).FindStringSubmatch(string(bc))
		if m == nil {
			t.Fatalf("no TRIVY entry for %s in build.conf", t.Want)
		}
		t.Contains(t.RunEntryAny("/usr/bin/trivy", "--version"), m[1], "bundled trivy version")
		if out, err := docker("run", "--rm", "--entrypoint", "/bin/sh", t.Image, "-c", "test -x /usr/bin/trivy && command -v trivy && test -x /usr/bin/scanner-trivy"); err != nil {
			t.Fatalf("trivy and scanner-trivy binaries\n%s", out)
		}
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw", "--tmpfs", "/home/scanner/.cache:rw,uid=1001,gid=1001",
			"-p", "127.0.0.1:18089:8080", "-e", "SCANNER_API_SERVER_ADDR=:8080", "-e", "SCANNER_LOG_LEVEL=info"})
		t.waitFor(60, time.Second, name, "/probe/healthy did not answer 200", func() bool {
			if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
				t.Fatalf("container exited early\n%s", t.Logs(name))
			}
			c, _ := Get("http://127.0.0.1:18089/probe/healthy")
			return c == 200
		})
		t.Logf("smoke test passed (harbor-trivy-adapter %s: trivy %s bundled, /probe/healthy 200 on a read-only rootfs; user %s)", t.Want, m[1], u)
	})
}
