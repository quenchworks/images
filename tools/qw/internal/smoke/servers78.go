package smoke

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func init() {
	// The default command runs pilot-discovery against a baked-in unreachable kubeconfig.
	// Measured: :8080 (debug/ready) binds early, then the config controller blocks waiting for
	// the ConfigMap cache sync; :15014 and :9876 only bind after that sync, so cluster-less they
	// never answer. Assert the live surface: the Pilot Debug Console index (rendered from the xDS
	// debug handler registry), /ready answering (503, not refused), and "waiting for sync" in
	// the log. --registries=Mock SIGSEGVs (serviceentry controller needs a kube client).
	register("istiod", func(t *T) {
		const base = "http://127.0.0.1:18080"
		u := t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"-p", "127.0.0.1:18080:8080"})
		var dbg string
		t.waitFor(60, time.Second, name, "discovery HTTP server (:8080) did not come up", func() bool {
			c, b := Get(base + "/debug")
			dbg = b
			return c == 200
		})
		for _, w := range []string{"Pilot Debug Console", "/debug/adsz", "/debug/syncz", "/debug/configz"} {
			t.Contains(dbg, w, "/debug console")
		}
		// /ready is 503 without a cluster; any HTTP answer proves the listener
		if c, _ := Get(base + "/ready"); c == 0 {
			t.Fatalf("/ready is not reachable\n%s", t.Logs(name))
		}
		// the first "waiting for sync" comes after the controller's 50th attempt (~4.5s)
		t.waitFor(30, time.Second, name, "no 'waiting for sync': the config controller never started", func() bool {
			return strings.Contains(t.Logs(name), "waiting for sync")
		})
		if t.Docker("inspect", "-f", "{{.State.Status}}", name) != "running" {
			t.Fatalf("container is not running any more\n%s", t.Logs(name))
		}
		ver := t.Match(t.RunEntryAny("/usr/bin/pilot-discovery", "version", "-s"), `(?m)^client version: (\S+)`, "stamped version")
		if strings.HasPrefix(ver, "unknown") || (t.Want != "" && !strings.HasPrefix(ver, t.Want)) {
			t.Fatalf("version %q, expected %s", ver, t.Want)
		}
		t.Logf("smoke test passed (istiod %s: live /debug console + /ready, real config controller; user %s)", ver, u)
	})

	// kafka_run.sh runs exactly as the Strimzi operator starts it (args only, custom-config
	// volume, serviceaccount namespace file), with a fake Kubernetes API serving the two Secrets
	// the kafka-agent loads (cluster CA cert, node cert + PKCS8 RSA key) and the JMX exporter on.
	// Strimzi's readiness probe (204 only at broker state RUNNING), a KRaft topic round-trip and
	// kafka_server metrics on :9404 are the assertions.
	register("strimzi-kafka", func(t *T) {
		const host = "smoke-kafka-0" // kafka_run.sh takes the broker id from the hostname suffix
		u := t.User("1001")
		if ep := t.Docker("inspect", t.Image, "--format", "{{json .Config.Entrypoint}}"); ep != "null" && ep != "[]" {
			t.Fatalf("expected no entrypoint, got %s", ep)
		}
		caPEM, nodePEM, keyPEM := rsaCAAndLeaf(host)
		secret := func(name string, data map[string][]byte) []byte {
			d := map[string]string{}
			for k, v := range data {
				d[k] = base64.StdEncoding.EncodeToString(v)
			}
			b, _ := json.Marshal(map[string]any{"apiVersion": "v1", "kind": "Secret", "type": "Opaque",
				"metadata": map[string]string{"name": name, "namespace": "default"}, "data": d})
			return b
		}
		secrets := map[string][]byte{
			"smoke-cluster-ca-cert": secret("smoke-cluster-ca-cert", map[string][]byte{"ca.crt": caPEM}),
			host:                    secret(host, map[string][]byte{host + ".crt": nodePEM, host + ".key": keyPEM}),
		}
		var hits []string
		api := t.hostStub(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits = append(hits, r.Method+" "+r.URL.Path)
			if b, ok := secrets[strings.TrimPrefix(r.URL.Path, "/api/v1/namespaces/default/secrets/")]; ok && r.Method == "GET" {
				w.Header().Set("Content-Type", "application/json")
				w.Write(b)
				return
			}
			http.NotFound(w, r)
		}))

		w := t.TempDir()
		cfg, sa := filepath.Join(w, "cfg"), filepath.Join(w, "sa")
		os.MkdirAll(cfg, 0o755)
		os.MkdirAll(sa, 0o755)
		os.WriteFile(filepath.Join(sa, "namespace"), []byte("default\n"), 0o644)
		files := map[string]string{
			"server.config": `node.id=0
process.roles=broker,controller
controller.listener.names=CONTROLLER
controller.quorum.voters=0@localhost:9090
listeners=CONTROLLER://:9090,PLAINTEXT://:9092
advertised.listeners=PLAINTEXT://localhost:9092
listener.security.protocol.map=CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT
inter.broker.listener.name=PLAINTEXT
log.dirs=/var/lib/kafka/data-0/kafka-log0
metadata.log.dir=/var/lib/kafka/data-0/kafka-log0
offsets.topic.replication.factor=1
transaction.state.log.replication.factor=1
transaction.state.log.min.isr=1
group.initial.rebalance.delay.ms=0
`,
			"cluster.id":       base64.RawURLEncoding.EncodeToString([]byte(randFrom("abcdefghijklmnop", 16))) + "\n",
			"metadata.version": "4.3-IV0\n",
			"log4j2.properties": "name=smoke\nappender.console.type=Console\nappender.console.name=STDOUT\nappender.console.layout.type=PatternLayout\n" +
				"appender.console.layout.pattern=%d %p %c - %m%n\nrootLogger.level=INFO\nrootLogger.appenderRefs=console\nrootLogger.appenderRef.console.ref=STDOUT\n",
			"metrics-config.json": `{"lowercaseOutputName": true, "rules": [{"pattern": ".*"}]}` + "\n",
			// Strimzi 1.3.0+ reads the agent config from custom-config; 1.2.0 ignores this file
			"agent.config": "sslTrustStoreSecretName=smoke-cluster-ca-cert\nsslKeyStoreSecretName=" + host + "\nnamespace=default\n",
		}
		for n, c := range files {
			os.WriteFile(filepath.Join(cfg, n), []byte(c), 0o644)
		}

		name := t.Name("")
		t.Start(name, append(append([]string{"--hostname", host}, hostStubFlag...),
			"-e", "KUBERNETES_MASTER="+api, "-e", "KUBERNETES_AUTH_TRYKUBECONFIG=false", "-e", "KUBERNETES_AUTH_TRYSERVICEACCOUNT=false",
			"-e", "KAFKA_CLUSTER_NAME=smoke", "-e", "KAFKA_HEAP_OPTS=-Xms512m -Xmx512m", "-e", "KAFKA_JMX_EXPORTER_ENABLED=true",
			"-v", cfg+":/opt/kafka/custom-config:ro", "-v", sa+":/var/run/secrets/kubernetes.io/serviceaccount:ro",
			"--tmpfs", "/var/lib/kafka:uid=1001,gid=1001,mode=0755"), "/opt/kafka/kafka_run.sh")
		t.waitFor(90, 2*time.Second, name, "broker never became ready (fake api: "+strings.Join(hits, ", ")+")", func() bool {
			if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
				t.Fatalf("container exited\n%s\nfake api: %v", t.Logs(name), hits)
			}
			_, err := docker("exec", name, "/opt/kafka/kafka_readiness.sh")
			return err == nil
		})

		const b = "localhost:9092"
		x := func(args ...string) string {
			return t.Docker(append([]string{"exec", "-e", "LOG_DIR=/tmp", "-e", "KAFKA_OPTS=", name}, args...)...)
		}
		x("/opt/kafka/bin/kafka-topics.sh", "--bootstrap-server", b, "--create", "--topic", "smoke", "--partitions", "1", "--replication-factor", "1")
		p := exec.Command("docker", "exec", "-i", "-e", "LOG_DIR=/tmp", name, "/opt/kafka/bin/kafka-console-producer.sh", "--bootstrap-server", b, "--topic", "smoke")
		p.Stdin = strings.NewReader("hello-quenchworks\n")
		if out, err := p.CombinedOutput(); err != nil {
			t.Fatalf("produce: %v\n%s", err, out)
		}
		got := x("/opt/kafka/bin/kafka-console-consumer.sh", "--bootstrap-server", b, "--topic", "smoke", "--from-beginning", "--max-messages", "1", "--timeout-ms", "20000")
		t.Contains(got, "hello-quenchworks", "message round-trip")
		t.Match(x("curl", "-s", "http://localhost:9404/metrics"), `(?m)^kafka_server_`, "JMX exporter kafka_server_ metrics")
		kv, _ := docker("exec", name, "/opt/kafka-exporter/kafka_exporter", "--version")
		if !regexp.MustCompile(`version`).MatchString(kv) {
			t.Fatalf("kafka_exporter --version failed: %s", kv)
		}
		t.Logf("smoke test passed (strimzi-kafka %s: kafka_run.sh ready via the agent, KRaft round-trip, JMX metrics; user %s)", t.Want, u)
	})
}

// rsaCAAndLeaf returns a CA cert, a leaf cert for cn and the leaf's PKCS8 RSA key, the shape
// Strimzi's cluster CA issues.
func rsaCAAndLeaf(cn string) (caPEM, leafPEM, keyPEM []byte) {
	ck, _ := rsa.GenerateKey(rand.Reader, 2048)
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "smoke-ca"}, NotBefore: time.Now().Add(-time.Hour),
		NotAfter: time.Now().Add(48 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	cder, _ := x509.CreateCertificate(rand.Reader, ca, ca, &ck.PublicKey, ck)
	ca, _ = x509.ParseCertificate(cder)
	lk, _ := rsa.GenerateKey(rand.Reader, 2048)
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: cn}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(48 * time.Hour)}
	lder, _ := x509.CreateCertificate(rand.Reader, leaf, ca, &lk.PublicKey, ck)
	kder, _ := x509.MarshalPKCS8PrivateKey(lk)
	enc := func(typ string, b []byte) []byte { return pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: b}) }
	return enc("CERTIFICATE", cder), enc("CERTIFICATE", lder), enc("PRIVATE KEY", kder)
}
