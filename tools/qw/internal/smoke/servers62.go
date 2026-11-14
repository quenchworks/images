package smoke

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func init() {
	// The cluster operator launched as upstream's Deployment does (args only), with the API server a
	// TEST-NET blackhole (192.0.2.1) so it sits in its connect timeout with /healthy up; then the
	// topic operator, user operator and kafka-init each reach their Main.
	register("strimzi-operator", func(t *T) {
		u := t.User("1001")
		if ep := t.Docker("inspect", t.Image, "--format", "{{json .Config.Entrypoint}}"); ep != "null" && ep != "[]" {
			t.Fatalf("expected no entrypoint, got %s", ep)
		}
		const k = "q.example/kafka:x"
		// the operator needs an image for EXACTLY the Kafka versions it supports (kafka-versions.yaml)
		kv := "4.2.0 4.2.1 4.2.2 4.3.0 4.3.1 4.3.2"
		switch {
		case strings.HasPrefix(t.Want, "1.2."):
			kv = "4.2.0 4.2.1 4.3.0 4.3.1"
		case t.Want == "1.3.0":
			kv = "4.2.0 4.2.1 4.2.2 4.3.0 4.3.1"
		}
		var m []string
		for _, v := range strings.Fields(kv) {
			m = append(m, v+"="+k)
		}
		kmap := strings.Join(m, "\n")
		env := []string{"-e", "STRIMZI_NAMESPACE=default", "-e", "STRIMZI_OPERATOR_NAMESPACE=default",
			"-e", "STRIMZI_KAFKA_IMAGES=" + kmap, "-e", "STRIMZI_KAFKA_CONNECT_IMAGES=" + kmap, "-e", "STRIMZI_KAFKA_MIRROR_MAKER_2_IMAGES=" + kmap,
			"-e", "KUBERNETES_SERVICE_HOST=192.0.2.1", "-e", "KUBERNETES_SERVICE_PORT=443"}
		for _, d := range []string{"KAFKA_EXPORTER_IMAGE", "CRUISE_CONTROL_IMAGE", "TOPIC_OPERATOR_IMAGE", "USER_OPERATOR_IMAGE",
			"KAFKA_INIT_IMAGE", "KAFKA_BRIDGE_IMAGE", "KANIKO_EXECUTOR_IMAGE", "BUILDAH_IMAGE", "MAVEN_BUILDER"} {
			env = append(env, "-e", "STRIMZI_DEFAULT_"+d+"="+k)
		}
		name := t.Name("")
		t.Start(name, append([]string{"-p", "127.0.0.1::8080"}, env...), "/opt/strimzi/bin/cluster_operator_run.sh")
		base := "http://127.0.0.1:" + t.hostPort(name, "8080/tcp")
		t.waitFor(60, time.Second, name, "cluster operator never served /healthy", func() bool { c, _ := Get(base + "/healthy"); return c == 204 })
		logs := t.Logs(name)
		t.Match(logs, "ClusterOperator "+regexp.QuoteMeta(t.Want)+".* is starting", "cluster operator start line")
		t.Contains(logs, "Health and metrics server is ready on port 8080", "health server line")
		_, metrics := Get(base + "/metrics")
		t.Match(metrics, `(?m)^jvm_`, "jvm_ series on /metrics")
		main := map[string][2]string{
			"to":   {"topic_operator_run.sh", "TopicOperator"},
			"uo":   {"user_operator_run.sh", "UserOperator"},
			"init": {"kafka_init_run.sh", "Init-kafka"},
		}
		for suffix, s := range main {
			n := t.Name(suffix)
			t.Start(n, []string{"-e", "STRIMZI_NAMESPACE=default", "-e", "STRIMZI_KAFKA_BOOTSTRAP_SERVERS=192.0.2.1:9092",
				"-e", "STRIMZI_CA_CERT_NAME=x", "-e", "STRIMZI_CA_KEY_NAME=x", "-e", "STRIMZI_CA_NAMESPACE=default", "-e", "NODE_NAME=n",
				"-e", "KUBERNETES_SERVICE_HOST=192.0.2.1", "-e", "KUBERNETES_SERVICE_PORT=443"}, "/opt/strimzi/bin/"+s[0])
			re := regexp.MustCompile(s[1] + " " + regexp.QuoteMeta(t.Want) + ".* is starting")
			t.waitFor(30, time.Second, n, s[0]+" did not reach its Main", func() bool { return re.MatchString(t.Logs(n)) })
		}
		t.Logf("smoke test passed (strimzi-operator %s: cluster operator /healthy and /metrics, three components reach Main; user %s)", t.Want, u)
	})

	// Odyssey in front of the postgresql image: a pooled query, then a TLS query with the process
	// still up afterwards (OpenSSL 4 builds segfaulted on the first TLS client), and a plain client
	// refused under tls "require".
	register("odyssey", func(t *T) {
		const pgImg = "ghcr.io/quenchworks/images/postgresql:18.6"
		u := t.User("1001")
		v := t.Run("--version")
		if t.Want != "" {
			t.Contains(v, t.Want, "odyssey version")
		}
		net_, pg, od := t.Name("net"), t.Name("pg"), t.Name("od")
		conf := t.TempDir()
		write := func(tls string) {
			c := `daemonize no
log_to_stdout yes
log_format "%p %t %l [%i %s] (%c) %m\n"
unix_socket_dir "/tmp"
unix_socket_mode "0644"
workers 1
listen {
	host "*"
	port 6432
` + tls + `
}
storage "pg" {
	type "remote"
	host "` + pg + `"
	port 5432
}
database default {
	user default {
		authentication "none"
		storage "pg"
		storage_user "postgres"
		storage_password "smoke-pass"
		pool "session"
	}
}
`
			os.WriteFile(filepath.Join(conf, "odyssey.conf"), []byte(c), 0o644)
		}
		write(`	tls "disable"`)
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Docker("run", "-d", "--name", pg, "--network", net_, "-e", "POSTGRES_PASSWORD=smoke-pass", pgImg)
		t.Cleanup(func() { docker("rm", "-f", pg) })
		t.Start(od, []string{"--network", net_, "--read-only", "--tmpfs", "/tmp", "-v", conf + ":/etc/odyssey:ro"})
		psql := func(mode, q string) string {
			out, _ := docker("run", "--rm", "--network", net_, "--entrypoint", "psql", "-e", "PGSSLMODE="+mode, pgImg,
				"-h", od, "-p", "6432", "-U", "postgres", "-d", "postgres", "-tAc", q)
			return strings.TrimSpace(out)
		}
		t.waitFor(60, 2*time.Second, od, "query never came back through the pooler", func() bool {
			return strings.HasPrefix(psql("disable", "select 'through odyssey ' || current_setting('server_version_num')"), "through odyssey ")
		})
		crt, key := certPair(od)
		os.WriteFile(filepath.Join(conf, "tls.crt"), crt, 0o644)
		os.WriteFile(filepath.Join(conf, "tls.key"), key, 0o644)
		write("\ttls \"require\"\n\ttls_cert_file \"/etc/odyssey/tls.crt\"\n\ttls_key_file \"/etc/odyssey/tls.key\"")
		t.Docker("restart", od)
		t.waitFor(30, 2*time.Second, od, "TLS query never came back", func() bool { return psql("require", "select 'tls through odyssey'") == "tls through odyssey" })
		time.Sleep(2 * time.Second)
		if s := t.Docker("inspect", "-f", "{{.State.Running}} {{.RestartCount}}", od); s != "true 0" {
			t.Fatalf("odyssey died after a TLS client (%s)\n%s", s, t.Logs(od))
		}
		if psql("disable", "select 1") == "1" {
			t.Fatalf("plain client accepted with tls require")
		}
		t.Logf("smoke test passed (odyssey %s: pooled query, TLS query with the process still up, plain refused under require; user %s)", t.Want, u)
	})
}
