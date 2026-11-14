package smoke

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// datahubSibling is a sibling image at the version under test (GMS 1.8.0 never reports healthy
// after a 1.7 SystemUpdate), an env override, or the pinned fallback when no version is given.
func datahubSibling(t *T, app, env, pinned string) string {
	if v := os.Getenv(env); v != "" {
		return v
	}
	if t.Want != "" {
		return "ghcr.io/quenchworks/images/" + app + ":" + t.Want
	}
	return "ghcr.io/quenchworks/images/" + app + "@" + pinned
}

// datahubGMS brings up MySQL, Kafka and Elasticsearch on a network, runs SystemUpdate, then GMS
// (gmsImage) on a read-only root published on gmsPort. It returns the network and system secret.
func datahubGMS(t *T, gmsImage, esPort, gmsPort string) (string, string) {
	const (
		mysqlImg = "ghcr.io/quenchworks/images/mysql@sha256:40b6296bb2cea66b225f8d26bbecd4a11859dce69e48a7267fcfd38e58ac052e"
		kafkaImg = "ghcr.io/quenchworks/images/kafka@sha256:6b7ea2f5c99d48e4ad40f686464687791b50e55d49b6b74b04985f38573febd2"
		esImg    = "ghcr.io/quenchworks/images/elasticsearch@sha256:f2b65a8c303e1591bcfde5a62435b0b97d60fc7a4a1399ced11ad8eb479ecda6"
	)
	upgrade := datahubSibling(t, "datahub-upgrade", "DATAHUB_UPGRADE_IMAGE", "sha256:7d4452a171f27fc059ff306181ea22c8aa13fb057579e76842788d41095a1576")
	net_, pw, sys := t.Name("net"), "db-"+hexRand(6), "sys-"+hexRand(6)
	t.Docker("network", "create", net_)
	t.Cleanup(func() { docker("network", "rm", net_) })
	run := func(name string, flags []string, img string, args ...string) {
		t.Docker(append(append(append([]string{"run", "-d", "--name", name, "--network", net_}, flags...), img), args...)...)
		t.Cleanup(func() { docker("rm", "-f", name) })
	}
	my, kf, es := t.Name("mysql"), t.Name("kafka"), t.Name("es")
	run(my, []string{"--network-alias", "mysql", "-e", "MYSQL_ROOT_PASSWORD=" + pw, "-e", "MYSQL_DATABASE=datahub"}, mysqlImg)
	run(kf, []string{"--network-alias", "kafka", "-e", "KAFKA_ADVERTISED_LISTENERS=PLAINTEXT://kafka:9092"}, kafkaImg)
	run(es, []string{"--network-alias", "search", "-p", "127.0.0.1:" + esPort + ":9200", "-e", "ES_JAVA_OPTS=-Xms512m -Xmx512m"}, esImg)
	t.waitFor(90, 2*time.Second, es, "dependencies did not become ready", func() bool {
		if _, err := docker("exec", my, "mysqladmin", "--socket=/run/mysqld/mysqld.sock", "-uroot", "-p"+pw, "ping"); err != nil {
			return false
		}
		if _, err := docker("exec", kf, "/opt/kafka/bin/kafka-broker-api-versions.sh", "--bootstrap-server", "localhost:9092"); err != nil {
			return false
		}
		c, _ := Get("http://127.0.0.1:" + esPort + "/")
		return c == 200
	})
	dh := []string{"-e", "EBEAN_DATASOURCE_USERNAME=root", "-e", "EBEAN_DATASOURCE_PASSWORD=" + pw, "-e", "EBEAN_DATASOURCE_HOST=mysql:3306",
		"-e", "EBEAN_DATASOURCE_URL=jdbc:mysql://mysql:3306/datahub?verifyServerCertificate=false&useSSL=true&useUnicode=yes&characterEncoding=UTF-8&enabledTLSProtocols=TLSv1.2",
		"-e", "EBEAN_DATASOURCE_DRIVER=com.mysql.jdbc.Driver", "-e", "GRAPH_SERVICE_IMPL=elasticsearch", "-e", "ELASTICSEARCH_HOST=search",
		"-e", "ELASTICSEARCH_PORT=9200", "-e", "ELASTICSEARCH_USE_SSL=false", "-e", "KAFKA_BOOTSTRAP_SERVER=kafka:9092",
		"-e", "SCHEMA_REGISTRY_TYPE=INTERNAL", "-e", "KAFKA_SCHEMAREGISTRY_URL=http://gms:8080/schema-registry/api/",
		"-e", "DATAHUB_SYSTEM_CLIENT_SECRET=" + sys, "-e", "DATAHUB_TOKEN_SERVICE_SIGNING_KEY=sign-" + hexRand(16),
		"-e", "DATAHUB_TOKEN_SERVICE_SALT=salt-" + hexRand(4), "-e", "JDK_JAVA_OPTIONS=-Xmx1g"}
	up := t.Name("up")
	t.Cleanup(func() { docker("rm", "-f", up) })
	upArgs := append(append([]string{"--read-only", "--tmpfs", "/tmp:exec"}, dh...), "-e", "USE_CONFLUENT_SCHEMA_REGISTRY=false",
		"-e", "DATAHUB_PRECREATE_TOPICS=true", "-e", "DATAHUB_SQL_SETUP_ENABLED=true", "-e", "PARTITIONS=1")
	out, _ := docker(append(append(append([]string{"run", "--name", up, "--network", net_}, upArgs...), upgrade), "-u", "SystemUpdate")...)
	if !strings.Contains(out, "Upgrade SystemUpdate completed with result SUCCEEDED") {
		t.Fatalf("SystemUpdate failed\n%s", tail(out, 40))
	}
	// Kafka's snappy and zstd natives unpack into /tmp and load from there: it must allow exec.
	// The consumers are off unless enabled (upstream compose and helm turn them on).
	gms := t.Name("gms")
	run(gms, append(append([]string{"--network-alias", "gms", "--read-only", "--tmpfs", "/tmp:exec", "-p", "127.0.0.1:" + gmsPort + ":8080"}, dh...),
		"-e", "METADATA_SERVICE_AUTH_ENABLED=true", "-e", "DATAHUB_TELEMETRY_ENABLED=false",
		"-e", "MAE_CONSUMER_ENABLED=true", "-e", "MCE_CONSUMER_ENABLED=true", "-e", "PE_CONSUMER_ENABLED=true"), gmsImage)
	t.waitFor(120, 3*time.Second, gms, "GMS never became healthy", func() bool {
		if t.Docker("inspect", "-f", "{{.State.Running}}", gms) != "true" {
			t.Fatalf("GMS exited\n%s", tail(t.Logs(gms), 40))
		}
		c, _ := Get("http://127.0.0.1:" + gmsPort + "/health")
		return c == 200
	})
	return net_, sys
}

func tail(s string, n int) string {
	l := strings.Split(s, "\n")
	if len(l) > n {
		l = l[len(l)-n:]
	}
	return strings.Join(l, "\n")
}

const datahubURN = "urn:li:dataset:(urn:li:dataPlatform:hive,quench.smoke,PROD)"

// datahubIngest upserts the smoke dataset through GMS's REST.li ingestProposal API.
func datahubIngest(t *T, gms, auth string) {
	body := fmt.Sprintf(`{"proposal":{"entityType":"dataset","entityUrn":%q,"changeType":"UPSERT","aspectName":"datasetProperties","aspect":{"value":"{\"name\":\"quench-smoke\",\"description\":\"from the image smoke test\"}","contentType":"application/json"}}}`, datahubURN)
	_, res := DoH("POST", gms+"/aspects?action=ingestProposal", body, map[string]string{"Authorization": auth,
		"Content-Type": "application/json", "X-RestLi-Protocol-Version": "2.0.0"})
	t.Contains(res, `"value":"success"`, "ingestProposal")
}

var datahubDatasetQuery = `{"query":"{ dataset(urn: \"` + datahubURN + `\") { properties { name description } } }"}`

func init() {
	// MySQL, Kafka and Elasticsearch, SystemUpdate, then GMS on a read-only root: healthy, its
	// version in /config, anonymous GraphQL refused, a dataset ingested over REST.li, read back over
	// GraphQL, and found by search (only once the MAE consumer has carried it to the index).
	register("datahub-gms", func(t *T) {
		const gms = "http://127.0.0.1:18088"
		u := t.User("1001")
		_, sys := datahubGMS(t, t.Image, "19202", "18088")
		if t.Want != "" {
			_, cfg := Get(gms + "/config")
			t.Contains(cfg, `"version" : "v`+t.Want+`"`, "GMS version in /config")
		}
		if c, _ := DoH("POST", gms+"/api/graphql", `{"query":"{ me { corpUser { urn } } }"}`, map[string]string{"Content-Type": "application/json"}); c != 401 {
			t.Fatalf("anonymous GraphQL answered %d, expected 401", c)
		}
		auth := "Basic __datahub_system:" + sys
		datahubIngest(t, gms, auth)
		h := map[string]string{"Authorization": auth, "Content-Type": "application/json"}
		_, got := DoH("POST", gms+"/api/graphql", datahubDatasetQuery, h)
		t.Contains(got, `"name":"quench-smoke"`, "dataset over GraphQL")
		var s string
		t.waitFor(30, 2*time.Second, t.Name("gms"), "dataset never reached the search index", func() bool {
			_, s = DoH("POST", gms+"/api/graphql", `{"query":"{ search(input:{type:DATASET, query:\"quench\", start:0, count:5}) { total } }"}`, h)
			return strings.Contains(s, `"total":1`)
		})
		t.Logf("smoke test passed (datahub-gms %s: dataset ingested, read back over GraphQL, found by search; user %s)", t.Want, u)
	})

	// The whole stack, then the frontend on a read-only root with a generated admin password: the
	// upstream default login is refused, the generated one accepted, the session mints a token
	// through the frontend's GraphQL proxy, the token ingests into GMS, and the dataset reads back
	// through the frontend.
	register("datahub-frontend", func(t *T) {
		const gms, fe = "http://127.0.0.1:18089", "http://127.0.0.1:19002"
		u := t.User("1001")
		gmsImg := datahubSibling(t, "datahub-gms", "DATAHUB_GMS_IMAGE", "sha256:5cb81778e07034d80c66cdbf95edbd48b8ddd9671f3cfa53f68fbc03dda3d615")
		net_, sys := datahubGMS(t, gmsImg, "19203", "18089")
		admin := "adm-" + hexRand(6)
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "user.props"), []byte("datahub:"+admin+"\n"), 0o644)
		name := t.Name("fe")
		t.Start(name, []string{"--network", net_, "--read-only", "--tmpfs", "/tmp", "-p", "127.0.0.1:19002:9002",
			"-v", dir + "/user.props:/datahub-frontend/conf/user.props:ro", "-e", "DATAHUB_GMS_HOST=gms", "-e", "DATAHUB_GMS_PORT=8080",
			"-e", "DATAHUB_SYSTEM_CLIENT_SECRET=" + sys, "-e", "DATAHUB_SECRET=" + hexRand(32), "-e", "KAFKA_BOOTSTRAP_SERVER=kafka:9092",
			"-e", "ELASTIC_CLIENT_HOST=search", "-e", "ELASTIC_CLIENT_PORT=9200", "-e", "JDK_JAVA_OPTIONS=-Xmx512m"})
		t.waitFor(90, 2*time.Second, name, "frontend never answered", func() bool {
			if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
				t.Fatalf("frontend exited\n%s", tail(t.Logs(name), 30))
			}
			c, _ := Get(fe + "/admin")
			return c == 200
		})
		login := func(pass string) (int, string) {
			r, err := noRedirect.Post(fe+"/logIn", "application/json", strings.NewReader(`{"username":"datahub","password":"`+pass+`"}`))
			if err != nil {
				return 0, ""
			}
			r.Body.Close()
			var ck []string
			for _, c := range r.Cookies() {
				ck = append(ck, c.Name+"="+c.Value)
			}
			return r.StatusCode, strings.Join(ck, "; ")
		}
		if c, _ := login("datahub"); c == 200 {
			t.Fatalf("the upstream default password datahub:datahub was accepted")
		}
		c, jar := login(admin)
		if c != 200 {
			t.Fatalf("login with the generated password answered %d\n%s", c, tail(t.Logs(name), 20))
		}
		gql := func(q string) string {
			_, b := DoH("POST", fe+"/api/v2/graphql", q, map[string]string{"Content-Type": "application/json", "Cookie": jar})
			return b
		}
		m := regexp.MustCompile(`"accessToken":"([^"]+)"`).FindStringSubmatch(gql(`{"query":"mutation { createAccessToken(input:{type:PERSONAL, actorUrn:\"urn:li:corpuser:datahub\", duration:ONE_HOUR, name:\"smoke\"}) { accessToken } }"}`))
		if m == nil {
			t.Fatalf("no access token minted")
		}
		datahubIngest(t, gms, "Bearer "+m[1])
		t.Contains(gql(datahubDatasetQuery), `"name":"quench-smoke"`, "dataset through the frontend")
		if regexp.MustCompile(` ERROR `).MatchString(t.Logs(name)) {
			t.Fatalf("frontend logged errors\n%s", tail(t.Logs(name), 30))
		}
		t.Logf("smoke test passed (datahub-frontend %s: default login refused, generated login, token minted, dataset ingested and read back; user %s)", t.Want, u)
	})
}
