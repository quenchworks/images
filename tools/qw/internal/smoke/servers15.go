package smoke

import (
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// waitFor polls cond once per gap up to tries; on timeout it fails with msg and the logs of name.
func (t *T) waitFor(tries int, gap time.Duration, name, msg string, cond func() bool) {
	for i := 0; i < tries; i++ {
		if cond() {
			return
		}
		time.Sleep(gap)
	}
	t.Fatalf("%s\n%s", msg, t.Logs(name))
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func init() {
	register("kafka-exporter", func(t *T) {
		u := t.User("1001")
		if v := t.RunAny("--version"); t.Want != "" {
			t.Contains(v, "version "+t.Want, "version")
		}
		kn, en := t.Name("broker"), t.Name("")
		t.Cleanup(func() { docker("rm", "-f", kn) })
		t.Docker("run", "-d", "--name", kn, "-p", "127.0.0.1:19308:9308", envOr("KAFKA_IMAGE", "ghcr.io/quenchworks/images/kafka:4.3.1"))
		t.waitFor(60, 2*time.Second, kn, "broker never started", func() bool { return strings.Contains(t.Logs(kn), "Kafka Server started") })
		t.Docker("exec", kn, "/opt/kafka/bin/kafka-topics.sh", "--bootstrap-server", "localhost:9092", "--create", "--topic", "quench-smoke", "--partitions", "2")
		t.Start(en, []string{"--network", "container:" + kn}, "--kafka.server=localhost:9092")
		var body string
		t.waitFor(30, time.Second, en, "kafka_brokers is not 1", func() bool {
			_, body = Get("http://127.0.0.1:19308/metrics")
			return regexp.MustCompile(`(?m)^kafka_brokers 1$`).MatchString(body)
		})
		t.Match(body, `(?m)^kafka_topic_partitions\{topic="quench-smoke"\} 2$`, "topic partitions scraped")
		t.Logf("smoke test passed (uid %s, 1 broker, topic partitions scraped)", u)
	})
	register("kafka", func(t *T) {
		const k, bs = "/opt/kafka/bin", "localhost:9092"
		name := t.Name("")
		t.Start(name, []string{"-e", "KAFKA_EXTRA_PROPERTIES=log.retention.hours=170"})
		t.waitFor(60, 2*time.Second, name, "kafka did not become ready", func() bool {
			_, err := docker("exec", name, k+"/kafka-broker-api-versions.sh", "--bootstrap-server", bs)
			return err == nil
		})
		t.Match(t.Docker("exec", name, "cat", "/var/lib/kafka/server.properties"), `(?m)^log\.retention\.hours=170$`, "KAFKA_EXTRA_PROPERTIES applied")
		t.Docker("exec", name, k+"/kafka-topics.sh", "--bootstrap-server", bs, "--create", "--topic", "smoke", "--partitions", "1", "--replication-factor", "1")
		t.Match(t.Docker("exec", name, k+"/kafka-topics.sh", "--bootstrap-server", bs, "--list"), `(?m)^smoke$`, "topic listed")
		if out, err := runHostIn("hello-quenchworks\n", "docker", "exec", "-i", name, k+"/kafka-console-producer.sh", "--bootstrap-server", bs, "--topic", "smoke"); err != nil {
			t.Fatalf("produce: %s", out)
		}
		out, _ := docker("exec", name, k+"/kafka-console-consumer.sh", "--bootstrap-server", bs, "--topic", "smoke", "--from-beginning", "--max-messages", "1", "--timeout-ms", "20000")
		t.Contains(out, "hello-quenchworks", "consumed")
		t.Logf("%s", t.Docker("exec", name, k+"/kafka-topics.sh", "--version"))
		t.Logf("smoke test passed (nonroot user: %s)", t.User("1001"))
	})
	register("opensearch-dashboards", func(t *T) {
		u := t.User("1001")
		osImg := envOr("OPENSEARCH_IMAGE", "ghcr.io/quenchworks/images/opensearch:"+map[bool]string{true: t.Want, false: "3.8.0"}[t.Want != ""])
		net_, os_, name := t.Name("net"), t.Name("os"), t.Name("")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Cleanup(func() { docker("rm", "-f", os_) })
		t.Docker("run", "-d", "--name", os_, "--network", net_, "--network-alias", "opensearch", osImg)
		t.Start(name, []string{"--network", net_, "--read-only", "--tmpfs", "/tmp", "--tmpfs", "/usr/share/opensearch-dashboards/data:rw,uid=1001,gid=1001", "-p", "127.0.0.1:15601:5601"})
		var st struct {
			Version struct{ Number string }
			Status  struct{ Overall struct{ State string } }
		}
		t.waitFor(120, 2*time.Second, name, "status never green", func() bool {
			_, b := Get("http://127.0.0.1:15601/api/status")
			return json.Unmarshal([]byte(b), &st) == nil && st.Status.Overall.State == "green"
		})
		if t.Want != "" && st.Version.Number != t.Want {
			t.Fatalf("version %s, expected %s", st.Version.Number, t.Want)
		}
		if c, b := DoH("POST", "http://127.0.0.1:15601/api/saved_objects/index-pattern/quench-smoke", `{"attributes":{"title":"quench-smoke-*"}}`,
			map[string]string{"osd-xsrf": "true", "Content-Type": "application/json"}); c/100 != 2 {
			t.Fatalf("saved object create: %d %s", c, b)
		}
		_, got := Get("http://127.0.0.1:15601/api/saved_objects/index-pattern/quench-smoke")
		t.Contains(got, `"title":"quench-smoke-*"`, "saved object read back")
		t.Logf("smoke test passed (opensearch-dashboards %s green, saved object round trip; uid %s)", st.Version.Number, u)
	})
	register("openziti", func(t *T) {
		v := t.RunAny("version")
		t.Match(v, `^v[0-9]+\.[0-9]+\.[0-9]+`, "version")
		if t.Want != "" && v != "v"+t.Want {
			t.Fatalf("version %s, expected v%s", v, t.Want)
		}
		name := t.Name("")
		pw := t.Name("pw")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp", "--tmpfs", "/home/ziti:uid=1001,mode=0750"},
			"edge", "quickstart", "--home", "/home/ziti/qs", "--ctrl-address", "127.0.0.1", "--router-address", "127.0.0.1", "-p", pw)
		t.waitFor(90, time.Second, name, "no edge router came online", func() bool {
			if s := t.Docker("inspect", "-f", "{{.State.Status}}", name); s != "running" {
				t.Fatalf("container died\n%s", t.Logs(name))
			}
			if _, err := docker("exec", name, "ziti", "edge", "login", "127.0.0.1:1280", "-u", "admin", "-p", pw, "-y"); err != nil {
				return false
			}
			r, _ := docker("exec", name, "ziti", "edge", "list", "edge-routers", "-j")
			return regexp.MustCompile(`"isOnline": *true`).MatchString(r)
		})
		if m := regexp.MustCompile(`(?i)read-only file system|permission denied|"level":"fatal"|panic:`).FindString(t.Logs(name)); m != "" {
			t.Fatalf("errors in the logs: %s", m)
		}
		t.Logf("smoke test passed (nonroot %s, read-only rootfs, controller + edge router online)", t.User("1001"))
	})
	register("ory-hydra", func(t *T) {
		u := t.User("1001")
		if v := t.RunAny("version"); t.Want != "" {
			t.Contains(v, t.Want, "version")
		}
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp", "-e", "DSN=memory", "-e", "SECRETS_SYSTEM=quenchworks-smoke-secret-0123456789",
			"-e", "URLS_SELF_ISSUER=http://127.0.0.1:14444", "-p", "127.0.0.1:14444:4444", "-p", "127.0.0.1:14445:4445"}, "serve", "all", "--dev")
		t.WaitHTTP("http://127.0.0.1:14445/health/ready", 60, name)
		t.WaitHTTP("http://127.0.0.1:14444/health/ready", 10, name)
		_, d := Get("http://127.0.0.1:14444/.well-known/openid-configuration")
		t.Contains(d, `"issuer":"http://127.0.0.1:14444`, "discovery document")
		_, cl := Post("http://127.0.0.1:14445/admin/clients", "application/json",
			`{"client_id":"smoke","client_secret":"smoke-secret-0123456789","grant_types":["client_credentials"],"token_endpoint_auth_method":"client_secret_basic"}`)
		t.Contains(cl, `"client_id":"smoke"`, "client create")
		req := url.Values{"grant_type": {"client_credentials"}}.Encode()
		_, tok := DoH("POST", "http://smoke:smoke-secret-0123456789@127.0.0.1:14444/oauth2/token", req, map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
		m := regexp.MustCompile(`"access_token":"([^"]+)"`).FindStringSubmatch(tok)
		if m == nil {
			t.Fatalf("no access token: %s", tok)
		}
		_, in := Post("http://127.0.0.1:14445/admin/oauth2/introspect", "application/x-www-form-urlencoded", url.Values{"token": {m[1]}}.Encode())
		t.Contains(in, `"active":true`, "token active on introspection")
		t.Logf("smoke test passed (uid %s, client_credentials token active)", u)
	})
	register("selenium-node-firefox", func(t *T) {
		u := t.User("1001")
		const base = "http://127.0.0.1:14445"
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp", "--tmpfs", "/home/nonroot:uid=1001,gid=1001", "--shm-size", "256m", "-p", "127.0.0.1:14445:4444"},
			"standalone", "--config", "/etc/selenium/config.toml")
		t.waitFor(90, time.Second, name, "grid never ready", func() bool {
			_, s := Get(base + "/status")
			return regexp.MustCompile(`"ready": *true`).MatchString(s)
		})
		if t.Want != "" {
			t.Contains(t.Logs(name), "Started Selenium Standalone "+t.Want, "standalone version")
		}
		j := map[string]string{"Content-Type": "application/json; charset=utf-8"}
		_, nw := DoH("POST", base+"/session", `{"capabilities":{"alwaysMatch":{"browserName":"firefox"}}}`, j)
		sid := regexp.MustCompile(`"sessionId": *"([^"]+)"`).FindStringSubmatch(nw)
		if sid == nil {
			t.Fatalf("no session created: %.800s\n%s", nw, t.Logs(name))
		}
		DoH("POST", base+"/session/"+sid[1]+"/url", `{"url":"data:text/html,<title>quench-selenium</title><h1>ok</h1>"}`, j)
		_, title := Get(base + "/session/" + sid[1] + "/title")
		t.Match(title, `"value": *"quench-selenium"`, "title read back")
		Do("DELETE", base+"/session/"+sid[1], "")
		t.Logf("smoke test passed (standalone firefox session, uid %s)", u)
	})
	register("skywalking", func(t *T) {
		u := t.User("1001")
		out, err := t.RunFlags([]string{"--entrypoint", "sh"}, "-c", `set -e
"$JAVA_HOME/bin/java" -version
ls /opt/skywalking/oap-libs/server-starter-*.jar >/dev/null
test -f /opt/skywalking/config/application.yml
sh -n /usr/bin/docker-entrypoint.sh
echo classpath-ok`)
		if err != nil || !strings.Contains(out, "classpath-ok") {
			t.Fatalf("JRE, classpath or config check failed:\n%s", out)
		}
		name := t.Name("")
		t.Start(name, nil)
		t.waitFor(60, time.Second, name, "OAP did not bind its gRPC receiver", func() bool {
			return regexp.MustCompile(`(?i)listening on 11800`).MatchString(t.Logs(name))
		})
		t.Logf("smoke test passed (nonroot uid: %s; OAP bound gRPC 11800)", u)
	})
	register("spark", func(t *T) {
		ro := []string{"--read-only", "--tmpfs", "/tmp", "--tmpfs", "/opt/spark/work-dir:uid=1001,gid=1001"}
		v, _ := t.RunFlags(ro, "/opt/spark/bin/spark-submit", "--version")
		if t.Want != "" {
			t.Contains(v, "version "+t.Want, "version")
		}
		pi, _ := t.RunFlags(ro, "/opt/spark/bin/spark-submit", "--master", "local[2]", "--class", "org.apache.spark.examples.SparkPi", "/opt/spark/examples/jars/spark-examples.jar", "20")
		t.Contains(pi, "Pi is roughly", "SparkPi")
		u := t.User("1001")
		net_, m, w := t.Name("net"), t.Name("master"), t.Name("worker")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Start(m, []string{"--network", net_, "--hostname", "master"}, "/opt/spark/bin/spark-class", "org.apache.spark.deploy.master.Master", "--host", "master", "--port", "7077", "--webui-port", "8080")
		t.Start(w, []string{"--network", net_, "-e", "SPARK_WORKER_DIR=/tmp/spark-worker"}, "/opt/spark/bin/spark-class", "org.apache.spark.deploy.worker.Worker", "spark://master:7077", "--cores", "1", "--memory", "512m")
		t.waitFor(60, 2*time.Second, m, "master never registered a worker", func() bool { return strings.Contains(t.Logs(m), "Registering worker") })
		t.Logf("smoke test passed (SparkPi, standalone master registered a worker; uid %s)", u)
	})
}

// runHostIn runs a host command with stdin.
func runHostIn(stdin, name string, args ...string) (string, error) {
	c := exec.Command(name, args...)
	c.Stdin = strings.NewReader(stdin)
	out, err := c.CombinedOutput()
	return strings.TrimRight(string(out), "\n"), err
}
