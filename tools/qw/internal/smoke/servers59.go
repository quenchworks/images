package smoke

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

func init() {
	// Drives nginx-proxy-manager end to end on a read-only root as uid 1001: the first admin from
	// INITIAL_ADMIN_*, /api/ reports OK and the release, an API login creates a proxy host
	// "smoke.test" forwarding to the admin server (the backend renders it, runs nginx -t and
	// reloads), and a request on the HTTP port with Host: smoke.test comes back through it.
	register("nginx-proxy-manager", func(t *T) {
		u := t.User("1001")
		data, le, name := t.Name("data"), t.Name("le"), t.Name("")
		for _, v := range []string{data, le} {
			t.Docker("volume", "create", v)
			vol := v
			t.Cleanup(func() { docker("volume", "rm", vol) })
			// a fresh named volume is root-owned; hand it to uid 1001 the way a pod fsGroup would
			t.Docker("run", "--rm", "-u", "0", "-v", v+":/d", "--entrypoint", "/usr/bin/node", t.Image, "-e", `require("fs").chownSync("/d",1001,1001)`)
		}
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp", "-v", data + ":/data", "-v", le + ":/etc/letsencrypt",
			"-e", "INITIAL_ADMIN_EMAIL=admin@smoke.test", "-e", "INITIAL_ADMIN_PASSWORD=quench-smoke-pw", "-e", "IP_RANGES_FETCH_ENABLED=false",
			"-p", "127.0.0.1::8181", "-p", "127.0.0.1::8080"})
		admin := "http://127.0.0.1:" + t.hostPort(name, "8181")
		web := "http://127.0.0.1:" + t.hostPort(name, "8080")
		var api string
		t.waitFor(120, time.Second, name, "API never reported OK", func() bool {
			_, api = Get(admin + "/api/")
			return strings.Contains(api, `"status":"OK"`)
		})
		t.Logf("api: %s", api)
		if t.Want != "" {
			p := strings.SplitN(t.Want, ".", 3)
			if len(p) == 3 {
				t.Contains(api, fmt.Sprintf(`"major":%s,"minor":%s,"revision":%s`, p[0], p[1], p[2]), "release version")
			}
		}
		js := map[string]string{"Content-Type": "application/json"}
		_, tb := DoH("POST", admin+"/api/tokens", `{"identity":"admin@smoke.test","secret":"quench-smoke-pw"}`, js)
		var tok struct{ Token string }
		if json.Unmarshal([]byte(tb), &tok); tok.Token == "" {
			t.Fatalf("no token: %.300s", tb)
		}
		auth := map[string]string{"Content-Type": "application/json", "Authorization": "Bearer " + tok.Token}
		_, cb := DoH("POST", admin+"/api/nginx/proxy-hosts",
			`{"domain_names":["smoke.test"],"forward_scheme":"http","forward_host":"127.0.0.1","forward_port":8181,"meta":{}}`, auth)
		var host struct{ ID int }
		if json.Unmarshal([]byte(cb), &host); host.ID == 0 {
			t.Fatalf("proxy host not created: %.300s", cb)
		}
		// the backend writes the config, tests it and reloads nginx after answering; it records
		// the outcome in the host's meta
		t.waitFor(30, time.Second, name, "proxy host not online and serving", func() bool {
			_, h := DoH("GET", fmt.Sprintf("%s/api/nginx/proxy-hosts/%d", admin, host.ID), "", auth)
			_, body := DoH("GET", web+"/", "", map[string]string{"Host": "smoke.test"})
			return strings.Contains(h, `"nginx_online":true`) && strings.Contains(strings.ToLower(body), `<div id="root"`)
		})
		t.Logf("Host: smoke.test on :8080 served the admin UI through the new proxy host")
		t.Logf("%s", t.Docker("exec", name, "certbot", "--version"))
		if m := regexp.MustCompile(`(?i)read-only file system|permission denied|EACCES|\[emerg\]`).FindString(t.Logs(name)); m != "" {
			t.Fatalf("errors in the logs (%s):\n%s", m, t.Logs(name))
		}
		t.Logf("smoke test passed (nonroot %s, read-only boot, API login, proxy host created and serving)", u)
	})
}

// gravMongo is our MongoDB 9 image: Docker Hub mongo:8.0 refuses to start on Linux 6.19+ kernels
// (the arm64 runners, 2026-10-19), and 9.0 has no such check. MONGO_IMAGE overrides it.
var gravMongo = envOr("MONGO_IMAGE", "ghcr.io/quenchworks/images/mongodb@sha256:8969ef1e59df01606d0255b5fd80eb8405c03fbdac5b595b4c97a9534c0a43ba")

// startMongo runs a throwaway MongoDB on net and waits for it to answer a ping; it returns
// the container name.
func (t *T) startMongo(net string) string {
	db := t.Name("mongo")
	t.Cleanup(func() { docker("rm", "-f", db) })
	t.Docker("run", "-d", "--name", db, "--network", net, "--read-only", "--tmpfs", "/data:rw,mode=1777,exec", gravMongo)
	t.waitFor(60, time.Second, db, "mongodb did not start", func() bool {
		_, err := docker("exec", "-e", "HOME=/data", db, "mongosh", "--quiet", "--eval", "db.runCommand({ping:1}).ok")
		return err == nil
	})
	return db
}

// gravLinkErr matches a class from a swapped jar failing to link.
var gravLinkErr = regexp.MustCompile(`NoClassDefFoundError|ClassNotFoundException|NoSuchMethodError|NoSuchFieldError|AbstractMethodError|IncompatibleClassChangeError|UnsupportedClassVersionError|Read-only file system`)

func init() {
	// The gateway against a throwaway MongoDB (management and rate-limit repository) on a read-only
	// root with only /tmp writable, OpenTelemetry tracing on so the swapped SDK initializes: its own
	// "no context-path" 404, a healthy node API, the plugins (mongodb repository included) loaded,
	// and no class from a swapped jar failing to link.
	register("gravitee-gateway", func(t *T) {
		u := t.User("1001")
		net, name := t.Name("net"), t.Name("")
		t.Docker("network", "create", net)
		t.Cleanup(func() { docker("network", "rm", net) })
		db := t.startMongo(net)
		uri := "mongodb://" + db + ":27017/gravitee"
		// 4.11 reads the older management.* / ratelimit.* keys, 4.12 the repositories.* ones
		t.Start(name, []string{"--network", net, "--read-only", "--tmpfs", "/tmp:rw,mode=1777",
			"-e", "gravitee_repositories_management_mongodb_uri=" + uri, "-e", "gravitee_repositories_ratelimit_mongodb_uri=" + uri,
			"-e", "gravitee_management_mongodb_uri=" + uri, "-e", "gravitee_ratelimit_mongodb_uri=" + uri,
			"-e", "gravitee_reporters_elasticsearch_enabled=false", "-e", "gravitee_services_core_http_host=0.0.0.0",
			"-e", "gravitee_services_opentelemetry_enabled=true", "-e", "gravitee_services_opentelemetry_exporter_endpoint=http://127.0.0.1:4317",
			"-p", "127.0.0.1::8082", "-p", "127.0.0.1::18082"})
		gw := "http://127.0.0.1:" + t.hostPort(name, "8082")
		node := "http://127.0.0.1:" + t.hostPort(name, "18082")
		var body string
		t.waitFor(120, time.Second, name, "gateway never answered", func() bool {
			if st := t.Docker("inspect", "-f", "{{.State.Status}}", name); st != "running" {
				t.Fatalf("gateway exited\n%s", t.Logs(name))
			}
			var code int
			code, body = Get(gw + "/")
			return code > 0 // any HTTP answer; 0 is a transport error
		})
		t.Contains(body, "No context-path matches the request URI", "gateway 404 page")
		n := t.gravNodeAndLogs(name, node)
		t.Contains(t.Logs(name), "Initializing service io.gravitee.node.opentelemetry.tracer.OpenTelemetryTracer", "OpenTelemetry tracer")
		t.Logf("smoke test passed (uid %s, gateway 404 page, node healthy, %d plugins)", u, n)
	})
}

// gravNodeAndLogs checks a Gravitee node: every health probe healthy (the node API starts after
// the main server), the release in /_node, the mongodb repository and >60 plugins loaded, and no
// class from a swapped jar failing to link or write hitting the read-only root. It returns the
// plugin count.
func (t *T) gravNodeAndLogs(name, node string) int {
	basic := map[string]string{"Authorization": "Basic YWRtaW46YWRtaW5hZG1pbg=="} // admin:adminadmin
	mgmtOK := regexp.MustCompile(`"management-repository"\s*:\s*\{\s*"healthy"\s*:\s*true`)
	down := regexp.MustCompile(`"healthy"\s*:\s*false`)
	var health string
	t.waitFor(120, time.Second, name, "node API not healthy", func() bool {
		_, health = DoH("GET", node+"/_node/health", "", basic)
		return mgmtOK.MatchString(health) && !down.MatchString(health)
	})
	t.Logf("health: %s", health)
	_, info := DoH("GET", node+"/_node", "", basic)
	if t.Want != "" {
		t.Match(info, `"MAJOR_VERSION"\s*:\s*"`+regexp.QuoteMeta(t.Want)+`"`, "release version")
	}
	logs := t.Logs(name)
	t.Contains(logs, "Repository [MANAGEMENT] loaded by mongodb", "mongodb repository plugin")
	n := strings.Count(logs, "has been loaded")
	t.Logf("plugins loaded: %d", n)
	if n <= 60 {
		t.Fatalf("only %d plugins loaded", n)
	}
	if m := gravLinkErr.FindString(logs); m != "" {
		t.Fatalf("a class failed to link or a write hit the read-only root (%s)\n%s", m, logs)
	}
	return n
}

func init() {
	// The management API against a throwaway MongoDB on a read-only root with only /tmp writable
	// and OpenTelemetry tracing on: the default admin gets a token, the token lists APIs on the v2
	// management API, the portal API answers, and the node checks above pass.
	register("gravitee-management-api", func(t *T) {
		u := t.User("1001")
		net, name := t.Name("net"), t.Name("")
		t.Docker("network", "create", net)
		t.Cleanup(func() { docker("network", "rm", net) })
		db := t.startMongo(net)
		uri := "mongodb://" + db + ":27017/gravitee"
		// 4.11 reads the older management.* keys, 4.12 the repositories.* ones: set both
		t.Start(name, []string{"--network", net, "--read-only", "--tmpfs", "/tmp:rw,mode=1777",
			"-e", "gravitee_repositories_management_mongodb_uri=" + uri, "-e", "gravitee_management_mongodb_uri=" + uri,
			"-e", "gravitee_repositories_analytics_type=none", "-e", "gravitee_analytics_type=none",
			"-e", "gravitee_services_core_http_host=0.0.0.0",
			"-e", "gravitee_services_opentelemetry_enabled=true", "-e", "gravitee_services_opentelemetry_exporter_endpoint=http://127.0.0.1:4317",
			"-p", "127.0.0.1::8083", "-p", "127.0.0.1::18083"})
		b := "http://127.0.0.1:" + t.hostPort(name, "8083")
		node := "http://127.0.0.1:" + t.hostPort(name, "18083")
		var tok, last string
		t.waitFor(180, time.Second, name, "admin login never returned a token", func() bool {
			if st := t.Docker("inspect", "-f", "{{.State.Status}}", name); st != "running" {
				t.Fatalf("management API exited\n%s", t.Logs(name))
			}
			_, last = DoH("POST", b+"/management/organizations/DEFAULT/user/login", "",
				map[string]string{"Authorization": "Basic YWRtaW46YWRtaW4="}) // admin:admin
			var r struct{ Token string }
			json.Unmarshal([]byte(last), &r)
			tok = r.Token
			return tok != ""
		})
		t.Logf("admin login ok")
		bearer := map[string]string{"Authorization": "Bearer " + tok}
		_, apis := DoH("GET", b+"/management/v2/environments/DEFAULT/apis", "", bearer)
		t.Contains(apis, `"data"`, "v2 API list")
		_, me := DoH("GET", b+"/management/organizations/DEFAULT/user", "", bearer)
		t.Match(me, `"displayName"\s*:\s*"admin"`, "current user admin")
		_, portal := Get(b + "/portal/environments/DEFAULT/configuration")
		t.Contains(portal, `"portal"`, "portal configuration")
		n := t.gravNodeAndLogs(name, node)
		t.Logf("smoke test passed (uid %s, admin token, v2 API list, portal, node healthy, %d plugins)", u, n)
	})
}
