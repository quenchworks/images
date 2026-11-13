package smoke

import (
	"crypto/rand"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var csrfToken = regexp.MustCompile(`name="_token" value="([^"]*)"`)

func init() {
	register("bookstack", func(t *T) {
		const db = "ghcr.io/quenchworks/images/mariadb@sha256:394b66879fa725ea4ab27d7b3beb6e556fcf0ee853241800d6eebf4ff7e7958d"
		const u = "http://127.0.0.1:18084"
		t.User("1001")
		ver := strings.TrimSpace(t.RunEntry("/bin/cat", "/var/www/bookstack/version"))
		t.Logf("bookstack %s", ver)
		if t.Want != "" && ver != "v"+t.Want {
			t.Fatalf("expected v%s", t.Want)
		}
		net_, dbn, app, pw := t.Name("net"), t.Name("db"), t.Name(""), t.Name("dbpw")
		st, up := t.Name("st"), t.Name("up")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Cleanup(func() { docker("volume", "rm", st, up) })
		t.Docker("run", "-d", "--name", dbn, "--network", net_, "--network-alias", "db", "-e", "MARIADB_ROOT_PASSWORD="+pw,
			"-e", "MARIADB_DATABASE=bookstack", "-e", "MARIADB_USER=bookstack", "-e", "MARIADB_PASSWORD="+pw, db)
		t.Cleanup(func() { docker("rm", "-f", dbn) })
		t.waitFor(60, time.Second, dbn, "mariadb never ready", func() bool {
			_, err := docker("exec", dbn, "mariadb", "-ubookstack", "-p"+pw, "-e", "select 1", "bookstack")
			return err == nil
		})
		k := make([]byte, 32)
		rand.Read(k)
		key := "base64:" + base64.StdEncoding.EncodeToString(k)
		start := func() {
			t.Start(app, []string{"--network", net_, "--read-only", "--tmpfs", "/tmp:uid=1001,gid=1001,mode=1777",
				"-v", st + ":/var/www/bookstack/storage", "-v", up + ":/var/www/bookstack/public/uploads",
				"-e", "APP_KEY=" + key, "-e", "APP_URL=" + u, "-e", "DB_HOST=db", "-e", "DB_DATABASE=bookstack",
				"-e", "DB_USERNAME=bookstack", "-e", "DB_PASSWORD=" + pw, "-p", "127.0.0.1:18084:8080"})
			t.waitFor(90, time.Second, app, "/login never served", func() bool {
				_, b := Get(u + "/login")
				return strings.Contains(b, "_token")
			})
		}
		// A browser session: cookie jar, CSRF token from each form, redirects not followed.
		var c *http.Client
		get := func(path string) string {
			r, err := c.Get(u + path)
			if err != nil {
				t.Fatalf("GET %s: %v", path, err)
			}
			defer r.Body.Close()
			b, _ := io.ReadAll(r.Body)
			return string(b)
		}
		post := func(path, formPage string, v url.Values) string {
			m := csrfToken.FindStringSubmatch(get(formPage))
			if m == nil {
				t.Fatalf("no CSRF token on %s", formPage)
			}
			v.Set("_token", m[1])
			r, err := c.PostForm(u+path, v)
			if err != nil {
				t.Fatalf("POST %s: %v", path, err)
			}
			r.Body.Close()
			return r.Header.Get("Location")
		}
		login := func(p string) string {
			jar, _ := cookiejar.New(nil)
			c = &http.Client{Jar: jar, Timeout: 30 * time.Second, CheckRedirect: noRedirect.CheckRedirect}
			return post("/login", "/login", url.Values{"email": {"admin@admin.com"}, "password": {p}})
		}
		book := func() string { login("password"); return get("/books/quench-smoke-book") }
		start()
		if r := login("wrong-password"); !strings.HasSuffix(r, "/login") {
			t.Fatalf("a wrong password redirected to %q", r)
		}
		if r := login("password"); r == "" || strings.HasSuffix(r, "/login") {
			t.Fatalf("admin login failed: %q\n%s", r, t.Logs(app))
		}
		post("/books", "/create-book", url.Values{"name": {"Quench Smoke Book"}, "description_html": {"<p>smoke</p>"}})
		t.Contains(book(), "Quench Smoke Book", "book readable")
		t.Logf("  migrated, wrong password refused, admin logged in, book created")
		docker("rm", "-f", app)
		start()
		t.Contains(book(), "Quench Smoke Book", "book kept across a restart")
		t.Logf("  book kept across a restart")
		if m := regexp.MustCompile(`(?i)read-only file system|PHP Fatal`).FindString(t.Logs(app)); m != "" {
			t.Fatalf("error in the logs (%s)\n%s", m, t.Logs(app))
		}
		t.Logf("smoke test passed (bookstack %s: migrate on MariaDB, web login with CSRF, book kept across a restart, read-only root; user 1001)", ver)
	})
	register("glance", func(t *T) {
		const u = "http://127.0.0.1:18080"
		t.User("1001")
		v := t.RunEntry("/usr/bin/glance", "--version")
		t.Logf("glance %s", strings.TrimSpace(v))
		t.WantVersion(v)
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "glance.yml"), []byte(`server:
  port: 8080
pages:
  - name: Home
    slug: home
    columns:
      - size: full
        widgets:
          - type: html
            source: <p id="quench-marker">quench-glance-smoke</p>
          - type: monitor
            title: Services
            sites:
              - title: Glance itself
                url: http://127.0.0.1:8080/api/healthz
`), 0o644)
		os.Chmod(dir, 0o755)
		name, _ := t.httpUp([]string{"--read-only", "--tmpfs", "/tmp:uid=1001,gid=1001", "-v", dir + ":/app/config:ro", "-p", "127.0.0.1:18080:8080"}, u+"/api/healthz", 30)
		_, page := Get(u + "/")
		t.Contains(page, "<html", "page HTML at /")
		var content string
		t.waitFor(20, time.Second, name, "widgets not rendered", func() bool {
			_, content = Get(u + "/api/pages/home/content/")
			return strings.Contains(content, "quench-glance-smoke") && strings.Contains(content, "Glance itself")
		})
		// The monitor template draws the status icon with --color-positive when the site is up.
		t.Contains(content, `fill="var(--color-positive)"`, "monitored site up")
		if strings.Contains(content, `fill="var(--color-negative)"`) {
			t.Fatalf("monitored site reported down")
		}
		t.Logf("  page content: HTML widget and monitor (site up) rendered")
		if m := regexp.MustCompile(`(?i)read-only file system|panic`).FindString(t.Logs(name)); m != "" {
			t.Fatalf("crashed or wrote to the root (%s)", m)
		}
		t.Logf("smoke test passed (glance %s: mounted config, HTML + monitor widgets rendered, read-only root; user 1001)", t.Want)
	})
	register("karapace", func(t *T) {
		const r, ct = "http://127.0.0.1:18081", "application/vnd.schemaregistry.v1+json"
		t.User("1001")
		net_ := t.Name("")
		kafka, reg, rest := net_+"-kafka", net_+"-reg", net_+"-rest"
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Cleanup(func() { docker("rm", "-f", kafka) })
		t.Docker("run", "-d", "--name", kafka, "--network", net_, "-e", "KAFKA_ADVERTISED_LISTENERS=PLAINTEXT://"+kafka+":9092",
			"ghcr.io/quenchworks/images/kafka:4.3.1")
		t.waitFor(60, 2*time.Second, kafka, "kafka did not become ready", func() bool {
			_, err := docker("exec", kafka, "/opt/kafka/bin/kafka-broker-api-versions.sh", "--bootstrap-server", "localhost:9092")
			return err == nil
		})
		t.Start(reg, []string{"--network", net_, "--read-only", "--tmpfs", "/tmp", "-e", "KARAPACE_BOOTSTRAP_URI=" + kafka + ":9092",
			"-e", "KARAPACE_ADVERTISED_HOSTNAME=" + reg, "-e", "KARAPACE_REPLICATION_FACTOR=1", "-p", "127.0.0.1:18081:8081"})
		t.waitCode(r+"/subjects", 90, time.Second, reg, 200)
		// Reads answer before the registry elects itself primary; writes 500 until then.
		avro := `{"schema":"{\"type\":\"record\",\"name\":\"Order\",\"fields\":[{\"name\":\"id\",\"type\":\"string\"}]}"}`
		t.waitFor(60, time.Second, reg, "avro register failed", func() bool {
			_, b := Post(r+"/subjects/orders-value/versions", ct, avro)
			return strings.Contains(b, `"id"`)
		})
		_, b := Post(r+"/subjects/orders-proto-value/versions", ct, `{"schemaType":"PROTOBUF","schema":"syntax = \"proto3\"; package q; message Order { string id = 1; int64 cents = 2; }"}`)
		t.Contains(b, `"id"`, "protobuf register")
		_, b = Post(r+"/compatibility/subjects/orders-proto-value/versions/latest", ct, `{"schemaType":"PROTOBUF","schema":"syntax = \"proto3\"; package q; message Order { int32 id = 1; }"}`)
		t.Contains(b, `"is_compatible":false`, "incompatible protobuf change rejected")
		_, b = Get(r + "/subjects")
		t.Contains(b, "orders-proto-value", "subjects")
		t.Start(rest, []string{"--network", net_, "--read-only", "--tmpfs", "/tmp", "-e", "KARAPACE_BOOTSTRAP_URI=" + kafka + ":9092",
			"-e", "KARAPACE_KARAPACE_REST=true", "-e", "KARAPACE_KARAPACE_REGISTRY=false", "-e", "KARAPACE_PORT=8082",
			"-e", "KARAPACE_REGISTRY_HOST=" + reg, "-e", "KARAPACE_REGISTRY_PORT=8081", "-p", "127.0.0.1:18082:8082",
			"--entrypoint", "/opt/karapace/venv/bin/python"}, "-m", "karapace.kafka_rest_apis")
		t.waitFor(60, time.Second, rest, "rest proxy did not list topics", func() bool {
			_, b := Get("http://127.0.0.1:18082/topics")
			return strings.Contains(b, "_schemas")
		})
		t.Logf("smoke test passed (karapace %s, uid 1001, avro + protobuf registry, compatibility, rest proxy)", t.Want)
	})
}
