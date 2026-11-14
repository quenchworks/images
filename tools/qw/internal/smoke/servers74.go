package smoke

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func init() {
	// No database: on a read-only rootfs with the chart's writable carve-outs, nginx and php-fpm
	// serve /status.php with the version, the front controller renders Nextcloud, occ runs, and
	// the required and recommended PHP extensions and runtime tools are present.
	register("nextcloud", func(t *T) {
		const base = "http://127.0.0.1:18087"
		u := t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777,uid=1001,gid=1001", "--tmpfs", "/var/www/html/data:rw,uid=1001,gid=1001",
			"--tmpfs", "/var/www/html/config:rw,uid=1001,gid=1001", "--tmpfs", "/var/www/html/custom_apps:rw,uid=1001,gid=1001", "-p", "127.0.0.1:18087:8080"})
		var status string
		t.waitFor(60, time.Second, name, "/status.php did not answer 200", func() bool {
			if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
				t.Fatalf("container exited early\n%s", t.Logs(name))
			}
			c, b := Get(base + "/status.php")
			status = b
			return c == 200
		})
		t.ContainsFold(status, "nextcloud", "/status.php")
		t.Contains(status, `"version"`, "/status.php version field")
		// pre-install the front controller renders setup but may answer 503
		c, idx := Get(base + "/")
		if c != 200 && c != 302 && c != 303 && c != 503 {
			t.Fatalf("front controller answered %d", c)
		}
		t.ContainsFold(idx, "nextcloud", "front controller body")
		v := t.Docker("exec", "-w", "/var/www/html", name, "php", "occ", "--version", "--no-warnings")
		if t.Want != "" {
			t.Contains(v, t.Want, "occ version")
		}
		mods := t.Docker("exec", name, "php", "-m")
		for _, ext := range strings.Fields("ctype curl dom gd fileinfo mbstring openssl posix simplexml xmlreader xmlwriter zip pdo pdo_mysql pdo_pgsql pdo_sqlite intl bcmath gmp exif sodium apcu redis imagick ldap") {
			t.Match(mods, `(?im)^`+ext+`$`, "PHP extension "+ext)
		}
		t.ContainsFold(mods, "opcache", "PHP extension opcache")
		for _, b := range []string{"php", "php-fpm", "nginx", "supervisord"} {
			if _, err := docker("exec", name, "sh", "-c", "command -v "+b); err != nil {
				t.Fatalf("runtime tool %s missing", b)
			}
		}
		t.Logf("smoke test passed (nextcloud %s: read-only rootfs, status.php, front controller, occ, extensions; user %s)", t.Want, u)
	})

	// Kafka traffic through the proxy: a read-only boot answers /livez and serves metrics, then a
	// broker from our kafka image sits behind Kroxylicious and the broker's own CLI creates a
	// topic, produces and consumes with only the proxy as bootstrap (it rewrites broker addresses).
	register("kroxylicious", func(t *T) {
		kafkaImg := envOr("KAFKA_IMAGE", "ghcr.io/quenchworks/images/kafka:4.3.2")
		const k = "/opt/kafka/bin"
		u := t.User("1001")
		v := t.Run("--version")
		t.Match(v, `(?m)^kroxylicious: `, "version line")
		if t.Want != "" {
			t.Match(v, `(?m)^kroxylicious: `+regexp.QuoteMeta(t.Want)+`$`, "kroxylicious version")
		}
		ro := t.Name("ro")
		t.Start(ro, []string{"--read-only", "--tmpfs", "/tmp:exec", "-p", "127.0.0.1::9190"})
		mb := "http://127.0.0.1:" + t.hostPort(ro, "9190/tcp")
		t.waitFor(60, time.Second, ro, "/livez never answered on a read-only rootfs", func() bool { c, _ := Get(mb + "/livez"); return c == 200 })
		_, metrics := Get(mb + "/metrics")
		t.Match(metrics, `(?m)^kroxylicious_build_info`, "kroxylicious metrics")
		if regexp.MustCompile(`(?i)read-only file system|permission denied|UnsatisfiedLinkError`).MatchString(t.Logs(ro)) {
			t.Fatalf("errors on a read-only rootfs\n%s", t.Logs(ro))
		}

		net_, kafka, proxy := t.Name("net"), t.Name("kafka"), t.Name("proxy")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Docker("run", "-d", "--name", kafka, "--network", net_, "-e", "KAFKA_EXTRA_PROPERTIES=advertised.listeners=PLAINTEXT://"+kafka+":9092", kafkaImg)
		t.Cleanup(func() { docker("rm", "-f", kafka) })
		apiv := func(bs string) bool {
			_, err := docker("exec", kafka, k+"/kafka-broker-api-versions.sh", "--bootstrap-server", bs)
			return err == nil
		}
		t.waitFor(60, 2*time.Second, kafka, "kafka did not become ready", func() bool { return apiv("localhost:9092") })
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(`management:
  endpoints:
    prometheus: {}
clusterDefinitions:
  - name: backend
    bootstrapServers: `+kafka+`:9092
virtualClusters:
  - name: smoke
    target:
      cluster: backend
    gateways:
      - name: default
        portIdentifiesNode:
          bootstrapAddress: 0.0.0.0:9192
          advertisedBrokerAddressPattern: `+proxy+`
`), 0o644)
		// a bind mount of a temp dir is refused under Docker Desktop and docker cp on a read-only
		// rootfs, so this part runs writable
		t.Docker("create", "--name", proxy, "--network", net_, t.Image)
		t.Cleanup(func() { docker("rm", "-f", proxy) })
		t.Docker("cp", filepath.Join(dir, "config.yaml"), proxy+":/etc/kroxylicious/config.yaml")
		t.Docker("start", proxy)
		bs := proxy + ":9192"
		t.waitFor(60, 2*time.Second, proxy, "the proxy never answered Kafka requests", func() bool { return apiv(bs) })
		t.Docker("exec", kafka, k+"/kafka-topics.sh", "--bootstrap-server", bs, "--create", "--topic", "smoke", "--partitions", "1", "--replication-factor", "1")
		t.Match(t.Docker("exec", kafka, k+"/kafka-topics.sh", "--bootstrap-server", bs, "--list"), `(?m)^smoke$`, "topic listed through the proxy")
		msg := "hello-through-kroxylicious-" + hexRand(4)
		p := exec.Command("docker", "exec", "-i", kafka, k+"/kafka-console-producer.sh", "--bootstrap-server", bs, "--topic", "smoke")
		p.Stdin = strings.NewReader(msg + "\n")
		if out, err := p.CombinedOutput(); err != nil {
			t.Fatalf("produce through the proxy: %v\n%s", err, out)
		}
		got := t.Docker("exec", kafka, k+"/kafka-console-consumer.sh", "--bootstrap-server", bs, "--topic", "smoke",
			"--from-beginning", "--max-messages", "1", "--timeout-ms", "30000")
		t.Match(got, `(?m)^`+msg+`$`, "message back through the proxy")
		t.Logf("smoke test passed (kroxylicious %s: read-only boot, /livez and metrics, Kafka produce/consume through the proxy; user %s)", t.Want, u)
	})
}
