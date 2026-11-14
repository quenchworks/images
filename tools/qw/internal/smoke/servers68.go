package smoke

import (
	"os"
	"path/filepath"
	"regexp"
	"time"
)

func init() {
	// otelcol-contrib on a read-only rootfs with an OTLP receiver, a debug exporter and the
	// health_check extension: :13133/ and the collector's own :8888/metrics answer 200.
	register("otel-collector", func(t *T) {
		u := t.User("1001")
		v := t.Run("--version")
		if t.Want != "" {
			t.Contains(v, t.Want, "otelcol-contrib version")
		}
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(`extensions:
  health_check:
    endpoint: 0.0.0.0:13133
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318
processors:
  batch:
exporters:
  debug:
    verbosity: basic
service:
  telemetry:
    metrics:
      readers:
        - pull:
            exporter:
              prometheus:
                host: 0.0.0.0
                port: 8888
  extensions: [health_check]
  pipelines:
    traces:
      receivers: [otlp]
      processors: [batch]
      exporters: [debug]
    metrics:
      receivers: [otlp]
      processors: [batch]
      exporters: [debug]
    logs:
      receivers: [otlp]
      processors: [batch]
      exporters: [debug]
`), 0o644)
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,uid=1001,gid=1001", "-v", dir + "/config.yaml:/etc/otelcol-contrib/config.yaml:ro",
			"-p", "127.0.0.1:13133:13133", "-p", "127.0.0.1:18888:8888"}, "--config=/etc/otelcol-contrib/config.yaml")
		t.waitFor(60, time.Second, name, ":13133 health_check did not become healthy", func() bool { c, _ := Get("http://127.0.0.1:13133/"); return c == 200 })
		if c, _ := Get("http://127.0.0.1:18888/metrics"); c != 200 {
			t.Fatalf(":8888/metrics returned %d\n%s", c, t.Logs(name))
		}
		t.Logf("smoke test passed (otel-collector %s: read-only rootfs, health_check and own metrics 200; user %s)", t.Want, u)
	})

	// HAProxy on a read-only rootfs with tmpfs for its admin socket: the monitor URI, the stats page
	// and an http frontend answered by `http-request return`.
	register("haproxy", func(t *T) {
		u := t.User("1001")
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "haproxy.cfg"), []byte(`global
    log stdout format raw local0
    maxconn 1024
    stats socket /var/lib/haproxy/admin.sock mode 660 level admin

defaults
    log     global
    mode    http
    option  httplog
    timeout connect 5s
    timeout client  50s
    timeout server  50s

frontend stats
    bind *:8404
    stats enable
    stats uri /stats
    monitor-uri /healthz

frontend http_in
    bind *:8080
    default_backend app

backend app
    http-request return status 200 content-type "text/plain" string "quench-haproxy ok"
`), 0o644)
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/var/lib/haproxy:rw,mode=1777", "--tmpfs", "/tmp:rw,mode=1777",
			"-v", dir + "/haproxy.cfg:/etc/haproxy/haproxy.cfg:ro", "-p", "127.0.0.1:18404:8404", "-p", "127.0.0.1:18080:8080"})
		t.waitFor(30, time.Second, name, "haproxy did not become ready", func() bool { c, _ := Get("http://127.0.0.1:18404/healthz"); return c == 200 })
		_, stats := Get("http://127.0.0.1:18404/stats")
		t.Match(stats, `(?i)Statistics Report|HAProxy`, "stats page")
		_, body := Get("http://127.0.0.1:18080/")
		t.Contains(body, "quench-haproxy ok", "http frontend")
		if regexp.MustCompile(`(?i)read-only file system|permission denied`).MatchString(t.Logs(name)) {
			t.Fatalf("read-only or permission errors in the logs\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed (haproxy %s: read-only rootfs, monitor URI, stats page, http frontend; user %s)", t.Want, u)
	})
}
