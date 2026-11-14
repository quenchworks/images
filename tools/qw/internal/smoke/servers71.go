package smoke

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func init() {
	// ATS answers only a working server could: its own 404 (Server: ATS/<version>) on a read-only
	// rootfs with tmpfs state dirs, then a front ATS reverse-proxies an origin ATS serving a file
	// through statichit, and the second request is a fresh cache hit (needs storage.config and the
	// cache dir right). Configs go in with docker cp, so that part runs on a writable rootfs.
	register("trafficserver", func(t *T) {
		u := t.User("1001")
		v := t.RunEntryAny("/usr/bin/traffic_server", "--version")
		t.Contains(v, "Apache Traffic Server - traffic_server - ", "version banner")
		if t.Want != "" {
			t.Contains(v, "traffic_server - "+t.Want+" -", "traffic_server version")
		}
		ro := t.Name("ro")
		t.Start(ro, []string{"--read-only", "--tmpfs", "/tmp", "--tmpfs", "/var/log/trafficserver:uid=1001",
			"--tmpfs", "/run/trafficserver:uid=1001", "--tmpfs", "/var/cache/trafficserver:uid=1001", "-p", "127.0.0.1::8080"})
		base := "http://127.0.0.1:" + t.hostPort(ro, "8080/tcp")
		var resp *http.Response
		t.waitFor(30, time.Second, ro, "ATS never answered on a read-only rootfs", func() bool {
			r, err := httpc.Get(base + "/")
			if err != nil {
				return false
			}
			r.Body.Close()
			resp = r
			return strings.HasPrefix(r.Header.Get("Server"), "ATS/")
		})
		if resp.StatusCode != 404 {
			t.Fatalf("expected 404 for an unmapped request, got %d", resp.StatusCode)
		}
		if t.Want != "" && !strings.HasPrefix(resp.Header.Get("Server"), "ATS/"+t.Want) {
			t.Fatalf("Server header %q, expected ATS/%s", resp.Header.Get("Server"), t.Want)
		}
		if regexp.MustCompile(`(?i)read-only file system|permission denied|FATAL|ALERT|EMERGENCY`).MatchString(t.Logs(ro)) {
			t.Fatalf("errors on a read-only rootfs\n%s", t.Logs(ro))
		}

		net_ := t.Name("net")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		dir := t.TempDir()
		marker := "quench-ats-smoke " + hexRand(4)
		os.WriteFile(filepath.Join(dir, "hello.txt"), []byte(marker+"\n"), 0o644)
		origin, front := t.Name("origin"), t.Name("front")
		start := func(name, remap string, extra ...string) {
			os.WriteFile(filepath.Join(dir, "remap.config"), []byte(remap+"\n"), 0o644)
			t.Docker(append(append([]string{"create", "--name", name, "--network", net_}, extra...), t.Image)...)
			t.Cleanup(func() { docker("rm", "-f", name) })
			t.Docker("cp", filepath.Join(dir, "remap.config"), name+":/etc/trafficserver/remap.config")
			if name == origin {
				t.Docker("cp", filepath.Join(dir, "hello.txt"), name+":/etc/trafficserver/hello.txt")
			}
			t.Docker("start", name)
		}
		start(origin, "map / http://127.0.0.1/ @plugin=statichit.so @pparam=--file-path=/etc/trafficserver/hello.txt @pparam=--mime-type=text/plain @pparam=--max-age=300")
		start(front, "map / http://"+origin+":8080/", "-p", "127.0.0.1::8080")
		fb := "http://127.0.0.1:" + t.hostPort(front, "8080/tcp")
		t.waitFor(30, time.Second, front, "the front never returned the origin's body", func() bool {
			r, err := httpc.Get(fb + "/")
			if err != nil {
				return false
			}
			b, _ := io.ReadAll(r.Body)
			r.Body.Close()
			return strings.Contains(string(b), marker)
		})
		Get(fb + "/")
		out := t.Docker("exec", front, "/usr/bin/traffic_ctl", "metric", "get", "proxy.process.http.cache_hit_fresh")
		f := strings.Fields(out)
		if n, _ := strconv.Atoi(f[len(f)-1]); n < 1 {
			t.Fatalf("no fresh cache hit (%s)", out)
		}
		t.Logf("smoke test passed (trafficserver %s: read-only 404 from ATS, reverse proxy, fresh cache hit; user %s)", t.Want, u)
	})

	// Mimir monolithic (-target=all) on a read-only rootfs with /data and /tmp on tmpfs: /ready and
	// /metrics 200. The relative default paths (activity tracker, compactor, ruler) are pinned
	// under /data, as a read-only cwd rejects them.
	register("mimir", func(t *T) {
		const base = "http://127.0.0.1:18085"
		u := t.User("1001")
		v := t.Run("-version")
		if t.Want != "" {
			t.Contains(v, t.Want, "mimir version")
		}
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "mimir.yaml"), []byte(`multitenancy_enabled: false
server:
  http_listen_port: 8080
  grpc_listen_port: 9095
activity_tracker:
  filepath: /data/metrics-activity.log
common:
  storage:
    backend: filesystem
    filesystem:
      dir: /data/storage
blocks_storage:
  backend: filesystem
  filesystem:
    dir: /data/blocks
  bucket_store:
    sync_dir: /data/tsdb-sync
  tsdb:
    dir: /data/tsdb
ruler_storage:
  backend: filesystem
  filesystem:
    dir: /data/rules
compactor:
  data_dir: /data/compactor
ruler:
  rule_path: /data/ruler
ingester:
  ring:
    replication_factor: 1
    kvstore:
      store: memberlist
usage_stats:
  enabled: false
`), 0o644)
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/data:rw,uid=1001,gid=1001", "--tmpfs", "/tmp:rw,uid=1001,gid=1001",
			"-v", dir + "/mimir.yaml:/etc/mimir/mimir.yaml:ro", "-p", "127.0.0.1:18085:8080"}, "-target=all", "-config.file=/etc/mimir/mimir.yaml")
		// Mimir answers 503 until every component has started
		t.waitFor(90, time.Second, name, "mimir /ready did not become ready", func() bool { c, _ := Get(base + "/ready"); return c == 200 })
		if c, _ := Get(base + "/metrics"); c != 200 {
			t.Fatalf("/metrics returned %d\n%s", c, t.Logs(name))
		}
		t.Logf("smoke test passed (mimir %s: monolithic on a read-only rootfs, /ready and /metrics 200; user %s)", t.Want, u)
	})
}
