package smoke

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

func init() {
	// Distroless: --version needs no config, then a real boot with a file backend and sqlite must
	// reach a healthy start (catches go:embed assets --version never touches).
	register("authelia", func(t *T) {
		u := t.User("1001")
		v, _ := t.RunFlags([]string{"--read-only"}, "--version")
		t.Match(v, `(?m)^authelia version v`, "authelia version line")
		if t.Want != "" {
			t.Match(v, `(?m)^authelia version v`+t.Want+`$`, "authelia version")
		}
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "configuration.yml"), []byte(`theme: light
server:
  address: tcp://0.0.0.0:9091
log:
  level: info
identity_validation:
  reset_password:
    jwt_secret: insecure_jwt_secret_for_smoke_test_only_0123456789abcdef
authentication_backend:
  file:
    path: /config/users.yml
access_control:
  default_policy: one_factor
session:
  secret: insecure_session_secret_for_smoke_test_only_0123456789
  cookies:
    - domain: example.com
      authelia_url: https://auth.example.com
storage:
  encryption_key: insecure_storage_encryption_key_for_smoke_test_only
  local:
    path: /config/db.sqlite3
notifier:
  filesystem:
    filename: /config/notification.txt
`), 0o666)
		os.WriteFile(filepath.Join(dir, "users.yml"), []byte(`users:
  smoke:
    disabled: false
    displayname: Smoke Test
    password: "$argon2id$v=19$m=65536,t=3,p=4$c21va2V0ZXN0c2FsdA$V1DVfUXlKV0KqmGmpsu4Ba1PjB47Vh4hTDqvbleyLE0"
    email: smoke@example.com
`), 0o666)
		name := t.Name("")
		t.Start(name, []string{"--read-only", "-v", dir + ":/config", "-p", "127.0.0.1:19091:9091"}, "--config", "/config/configuration.yml")
		t.waitFor(30, 2*time.Second, name, "authelia did not reach a healthy start", func() bool {
			if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
				t.Fatalf("authelia exited\n%s", t.Logs(name))
			}
			c, b := Get("http://127.0.0.1:19091/api/health")
			return c == 200 && strings.Contains(b, `"status":"OK"`)
		})
		t.Logf("smoke test passed (authelia %s: version, served with a file backend and sqlite, /api/health OK; user %s)", t.Want, u)
	})

	// Loki single-binary on a read-only rootfs with /loki and /tmp on tmpfs: /ready and /metrics 200.
	register("loki", func(t *T) {
		const base = "http://127.0.0.1:13100"
		u := t.User("1001")
		v := t.Run("-version")
		if t.Want != "" {
			t.Contains(v, t.Want, "loki version")
		}
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "loki.yaml"), []byte(`auth_enabled: false
server:
  http_listen_port: 3100
  grpc_listen_port: 9095
common:
  instance_addr: 127.0.0.1
  path_prefix: /loki
  storage:
    filesystem:
      chunks_directory: /loki/chunks
      rules_directory: /loki/rules
  replication_factor: 1
  ring:
    kvstore:
      store: inmemory
schema_config:
  configs:
    - from: 2020-10-24
      store: tsdb
      object_store: filesystem
      schema: v13
      index:
        prefix: index_
        period: 24h
analytics:
  reporting_enabled: false
`), 0o644)
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/loki:rw,uid=1001,gid=1001", "--tmpfs", "/tmp:rw,uid=1001,gid=1001",
			"-v", dir + "/loki.yaml:/etc/loki/loki.yaml:ro", "-p", "127.0.0.1:13100:3100"}, "-config.file=/etc/loki/loki.yaml")
		// Loki answers 503 until startup completes
		t.waitFor(60, time.Second, name, "loki /ready did not become ready", func() bool { c, _ := Get(base + "/ready"); return c == 200 })
		if c, _ := Get(base + "/metrics"); c != 200 {
			t.Fatalf("/metrics returned %d\n%s", c, t.Logs(name))
		}
		t.Logf("smoke test passed (loki %s: read-only rootfs, /ready and /metrics 200; user %s)", t.Want, u)
	})
}
