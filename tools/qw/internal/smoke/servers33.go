package smoke

import (
	"encoding/json"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

func init() {
	register("datahub-upgrade", func(t *T) {
		const (
			mysql = "ghcr.io/quenchworks/images/mysql@sha256:40b6296bb2cea66b225f8d26bbecd4a11859dce69e48a7267fcfd38e58ac052e"
			kafka = "ghcr.io/quenchworks/images/kafka@sha256:6b7ea2f5c99d48e4ad40f686464687791b50e55d49b6b74b04985f38573febd2"
			es    = "ghcr.io/quenchworks/images/elasticsearch@sha256:f2b65a8c303e1591bcfde5a62435b0b97d60fc7a4a1399ced11ad8eb479ecda6"
		)
		u := t.User("1001")
		net_, my, kf, sr, up := t.Name("net"), t.Name("mysql"), t.Name("kafka"), t.Name("es"), t.Name("up")
		pw := t.Name("db")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Cleanup(func() { docker("rm", "-f", my, kf, sr, up) })
		t.Docker("run", "-d", "--name", my, "--network", net_, "--network-alias", "mysql", "-e", "MYSQL_ROOT_PASSWORD="+pw, "-e", "MYSQL_DATABASE=datahub", mysql)
		t.Docker("run", "-d", "--name", kf, "--network", net_, "--network-alias", "kafka", "-e", "KAFKA_ADVERTISED_LISTENERS=PLAINTEXT://kafka:9092", kafka)
		t.Docker("run", "-d", "--name", sr, "--network", net_, "--network-alias", "search", "-p", "127.0.0.1:19201:9200", "-e", "ES_JAVA_OPTS=-Xms512m -Xmx512m", es)
		t.waitFor(90, 2*time.Second, sr, "dependencies did not become ready", func() bool {
			if _, err := docker("exec", my, "mysqladmin", "--socket=/run/mysqld/mysqld.sock", "-uroot", "-p"+pw, "ping"); err != nil {
				return false
			}
			if c, _ := Get("http://127.0.0.1:19201/"); c != 200 {
				return false
			}
			_, err := docker("exec", kf, "/opt/kafka/bin/kafka-broker-api-versions.sh", "--bootstrap-server", "localhost:9092")
			return err == nil
		})
		env := []string{"EBEAN_DATASOURCE_USERNAME=root", "EBEAN_DATASOURCE_PASSWORD=" + pw, "EBEAN_DATASOURCE_HOST=mysql:3306",
			"EBEAN_DATASOURCE_URL=jdbc:mysql://mysql:3306/datahub?verifyServerCertificate=false&useSSL=true&useUnicode=yes&characterEncoding=UTF-8&enabledTLSProtocols=TLSv1.2",
			"EBEAN_DATASOURCE_DRIVER=com.mysql.jdbc.Driver", "GRAPH_SERVICE_IMPL=elasticsearch", "ELASTICSEARCH_HOST=search", "ELASTICSEARCH_PORT=9200",
			"ELASTICSEARCH_USE_SSL=false", "KAFKA_BOOTSTRAP_SERVER=kafka:9092", "SCHEMA_REGISTRY_TYPE=INTERNAL", "KAFKA_SCHEMAREGISTRY_URL=http://gms:8080/schema-registry/api/",
			"USE_CONFLUENT_SCHEMA_REGISTRY=false", "DATAHUB_PRECREATE_TOPICS=true", "DATAHUB_SQL_SETUP_ENABLED=true", "PARTITIONS=1",
			"DATAHUB_SYSTEM_CLIENT_SECRET=" + t.Name("sys"), "DATAHUB_TOKEN_SERVICE_SIGNING_KEY=" + t.Name("sign") + "-0123456789abcdef0123456789",
			"DATAHUB_TOKEN_SERVICE_SALT=" + t.Name("salt"), "JDK_JAVA_OPTIONS=-Xmx768m"}
		flags := []string{"--name", up, "--network", net_, "--read-only", "--tmpfs", "/tmp:exec"}
		for _, e := range env {
			flags = append(flags, "-e", e)
		}
		out, code := t.RunAnyFor(480*time.Second, flags, "-u", "SystemUpdate")
		if code != 0 || !strings.Contains(out, "Upgrade SystemUpdate completed with result SUCCEEDED") {
			t.Fatalf("SystemUpdate failed (exit %d):\n%s", code, out)
		}
		tables, _ := docker("exec", my, "mysql", "--socket=/run/mysqld/mysqld.sock", "-uroot", "-p"+pw, "-N", "-e", "SHOW TABLES", "datahub")
		t.Match(tables, `(?m)^metadata_aspect_v2$`, "metadata_aspect_v2 table")
		topics := t.Docker("exec", kf, "/opt/kafka/bin/kafka-topics.sh", "--bootstrap-server", "localhost:9092", "--list")
		t.Match(topics, `(?m)^MetadataChangeLog_Versioned_v1$`, "MCL topic")
		_, idx := Get("http://127.0.0.1:19201/_cat/indices?h=index")
		t.Match(idx, `(?m)^datasetindex_v2$`, "dataset index")
		t.Logf("smoke test passed (uid %s, SystemUpdate SUCCEEDED: table, topics, indices)", u)
	})
	register("gradle", func(t *T) {
		if t.Want == "" || len(t.Args) == 0 {
			t.Fatalf("usage: qw smoke gradle <image-ref> <gradlemajor> <jdkmajor>")
		}
		gradle, jdk := t.Want, t.Args[0]
		ro := []string{"--read-only", "--tmpfs", "/tmp"}
		b, err := exec.Command("docker", append(append([]string{"run", "--rm"}, ro...), t.Image, "--version")...).Output()
		if err != nil {
			t.Fatalf("gradle --version returned non-zero: %v", err)
		}
		out := string(b) // stdout only: the daemon logs to stderr
		t.Contains(out, "Gradle "+gradle+".", "Gradle version")
		t.Contains(out, "Launcher JVM:  "+jdk+".", "launcher JVM")
		t.Contains(out, "/usr/lib/jvm/java-"+jdk+"-openjdk", "daemon JVM")
		jv, _ := t.RunFlags(append(ro, "--entrypoint", "/usr/bin/java"), "-version")
		t.Match(jv, `openjdk version "`+jdk+`(\.|")`, "bundled JDK")
		if jh, _ := t.RunFlags(append(ro, "--entrypoint", "/usr/bin/printenv"), "JAVA_HOME"); jh != "/usr/lib/jvm/java-"+jdk+"-openjdk" {
			t.Fatalf("JAVA_HOME is '%s'", jh)
		}
		t.User("1001")
		t.Contains(t.Docker("inspect", t.Image, "--format", "{{json .Config.Entrypoint}}"), "/usr/bin/gradle", "entrypoint")
		t.Logf("smoke test passed (Gradle %s on OpenJDK %s, read-only rootfs)", gradle, jdk)
	})
	register("grafana", func(t *T) {
		const b = "http://127.0.0.1:3000"
		name, h := t.httpUp([]string{"--read-only", "--tmpfs", "/var/lib/grafana:rw,mode=1777", "--tmpfs", "/var/log/grafana:rw,mode=1777", "--tmpfs", "/tmp:rw,mode=1777",
			"-e", "GF_SECURITY_ADMIN_PASSWORD=quenchtest", "-p", "127.0.0.1:3000:3000"}, b+"/api/health", 60)
		t.Match(h, `"database": *"ok"`, "database ok")
		if t.Want != "" {
			t.Match(h, `"version": *"`+regexp.QuoteMeta(t.Want)+`"`, "version")
		}
		_, login := Get(b + "/login")
		t.ContainsFold(login, "<html", "login page")
		t.ContainsFold(login, "grafana", "login page")
		_, org := Get("http://admin:quenchtest@127.0.0.1:3000/api/org")
		t.Contains(org, `"name"`, "default org with admin creds")
		if uid := t.Docker("exec", name, "id", "-u"); uid != "1001" {
			t.Fatalf("expected uid 1001, got %s", uid)
		}
		t.Logf("smoke test passed (health, login, admin API, uid 1001)")
	})
	register("jellyfin", func(t *T) {
		const b = "http://127.0.0.1:18096"
		u := t.Docker("inspect", t.Image, "--format", "{{.Config.User}}")
		t.Match(t.RunEntryAny("/usr/bin/ffmpeg", "-version"), `^ffmpeg version`, "ffmpeg present")
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp", "--tmpfs", "/config:uid=1001,gid=1001", "--tmpfs", "/cache:uid=1001,gid=1001", "-p", "127.0.0.1:18096:8096"})
		t.waitFor(90, time.Second, name, "never healthy", func() bool { _, h := Get(b + "/health"); return h == "Healthy" })
		_, info := Get(b + "/System/Info/Public")
		var si struct {
			Version                string
			StartupWizardCompleted bool
		}
		json.Unmarshal([]byte(info), &si)
		if si.Version == "" || (t.Want != "" && !strings.HasPrefix(si.Version, t.Want)) {
			t.Fatalf("jellyfin %q, expected %s", si.Version, t.Want)
		}
		if si.StartupWizardCompleted {
			t.Fatalf("not a fresh install: %s", info)
		}
		auth := map[string]string{"Content-Type": "application/json", "Authorization": `MediaBrowser Client="smoke", Device="test", DeviceId="smoke-1", Version="1"`}
		for _, s := range [][3]string{{"POST", "/Startup/Configuration", `{"UICulture":"en-US","MetadataCountryCode":"US","PreferredMetadataLanguage":"en"}`},
			{"GET", "/Startup/User", ""}, {"POST", "/Startup/User", `{"Name":"admin","Password":"smoke-pw-12345"}`}, {"POST", "/Startup/Complete", ""}} {
			if c, r := DoH(s[0], b+s[1], s[2], auth); c/100 != 2 {
				t.Fatalf("%s %s: %d %s", s[0], s[1], c, r)
			}
		}
		_, ar := DoH("POST", b+"/Users/AuthenticateByName", `{"Username":"admin","Pw":"smoke-pw-12345"}`, auth)
		var a struct{ AccessToken string }
		if json.Unmarshal([]byte(ar), &a) != nil || a.AccessToken == "" {
			t.Fatalf("no access token: %s", ar)
		}
		if c, _ := DoH("POST", b+"/Users/AuthenticateByName", `{"Username":"admin","Pw":"wrong"}`, auth); c != 401 {
			t.Fatalf("a wrong password returned %d, expected 401", c)
		}
		if regexp.MustCompile(`(?i)read-only file system|unauthorizedaccess`).MatchString(t.Logs(name)) {
			t.Fatalf("the server hit the read-only root\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed (jellyfin %s, read-only rootfs, user %s, ffmpeg present)", si.Version, u)
	})
}
