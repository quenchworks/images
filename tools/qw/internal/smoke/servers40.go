package smoke

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func init() {
	register("akhq", func(t *T) {
		const k = "/opt/kafka/bin"
		u := t.User("1001")
		net_, kf, name := t.Name("net"), t.Name("kafka"), t.Name("")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Cleanup(func() { docker("rm", "-f", kf) })
		t.Docker("run", "-d", "--name", kf, "--network", net_, "-e", "KAFKA_ADVERTISED_LISTENERS=PLAINTEXT://"+kf+":9092", "ghcr.io/quenchworks/images/kafka:4.3.1")
		t.waitFor(60, 2*time.Second, kf, "kafka did not become ready", func() bool {
			_, err := docker("exec", kf, k+"/kafka-broker-api-versions.sh", "--bootstrap-server", "localhost:9092")
			return err == nil
		})
		t.Docker("exec", kf, k+"/kafka-topics.sh", "--bootstrap-server", "localhost:9092", "--create", "--topic", "quench-smoke", "--partitions", "1", "--replication-factor", "1")
		w := t.TempDir()
		os.WriteFile(filepath.Join(w, "application.yml"), []byte("micronaut:\n  server:\n    port: 8080\nendpoints:\n  all:\n    port: 28081\nakhq:\n  connections:\n    smoke:\n      properties:\n        bootstrap.servers: \""+kf+":9092\"\n"), 0o644)
		t.Start(name, []string{"--network", net_, "--read-only", "--tmpfs", "/tmp", "-v", w + "/application.yml:/etc/akhq/application.yml:ro", "-p", "127.0.0.1:18080:8080", "-p", "127.0.0.1:28081:28081"})
		t.waitFor(90, time.Second, name, "akhq never reported UP", func() bool { _, h := Get("http://127.0.0.1:28081/health"); return strings.Contains(h, `"status":"UP"`) })
		t.WaitHTTP("http://127.0.0.1:18080/ui", 10, name)
		_, topics := Get("http://127.0.0.1:18080/api/smoke/topic?show=ALL")
		t.Contains(topics, `"name":"quench-smoke"`, "topic listed")
		if regexp.MustCompile(` ERROR |Exception`).MatchString(t.Logs(name)) {
			t.Fatalf("akhq logged errors\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed (uid %s, topic listed from a live broker)", u)
	})
	register("hasura", func(t *T) {
		const pgImg = "ghcr.io/quenchworks/images/postgresql@sha256:4c0580fd2968b8d6e8fb008e0eb74b55c54464b041c235842e4d1cc37ed9a445"
		const b = "http://127.0.0.1:18080"
		u := t.Docker("inspect", t.Image, "--format", "{{.Config.User}}")
		v := t.RunEntry("/usr/bin/graphql-engine", "version")
		t.Match(v, `-ce$`, "Community Edition")
		if t.Want != "" {
			t.Match(v, `v`+regexp.QuoteMeta(t.Want)+`-ce$`, "version")
		}
		net_, pg, hge := t.Name("net"), t.Name("pg"), t.Name("")
		pw, secret := t.Name("pg"), t.Name("admin")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Cleanup(func() { docker("rm", "-f", pg) })
		t.Docker("run", "-d", "--name", pg, "--network", net_, "--network-alias", "pg", "-e", "POSTGRES_PASSWORD="+pw, "-e", "POSTGRES_DB=hasura", pgImg)
		t.waitFor(60, time.Second, pg, "postgres never ready", func() bool {
			_, err := docker("exec", pg, "pg_isready", "-h", "127.0.0.1", "-U", "postgres", "-d", "hasura")
			return err == nil
		})
		t.Start(hge, []string{"--network", net_, "-p", "127.0.0.1:18080:8080", "-e", "HASURA_GRAPHQL_DATABASE_URL=postgres://postgres:" + pw + "@pg:5432/hasura",
			"-e", "HASURA_GRAPHQL_ADMIN_SECRET=" + secret})
		t.waitFor(90, 2*time.Second, hge, "/healthz never OK", func() bool { _, h := Get(b + "/healthz"); return h == "OK" })
		h := map[string]string{"X-Hasura-Admin-Secret": secret, "Content-Type": "application/json"}
		call := func(path, body string) string {
			c, r := DoH("POST", b+path, body, h)
			if c != 200 {
				t.Fatalf("%s: %d %s", path, c, r)
			}
			return r
		}
		call("/v2/query", `{"type":"run_sql","args":{"source":"default","sql":"CREATE TABLE smoke (id serial PRIMARY KEY, name text NOT NULL)"}}`)
		call("/v1/metadata", `{"type":"pg_track_table","args":{"source":"default","table":"smoke"}}`)
		call("/v1/graphql", `{"query":"mutation { insert_smoke_one(object: {name: \"quench\"}) { id } }"}`)
		t.Contains(call("/v1/graphql", `{"query":"{ smoke { name } }"}`), `"name":"quench"`, "row returned")
		_, anon := Post(b+"/v1/graphql", "application/json", `{"query":"{ smoke { name } }"}`)
		t.Contains(anon, `"errors"`, "anonymous request refused")
		t.Logf("smoke test passed (CE: run_sql, track, GraphQL insert and query, admin secret; user %s)", u)
	})
	register("homepage", func(t *T) {
		const b = "http://localhost:13000"
		w := t.TempDir()
		os.WriteFile(filepath.Join(w, "services.yaml"), []byte("- Quench Group:\n    - Quench Service:\n        href: http://quench.invalid/\n        description: smoke test entry\n"), 0o666)
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-e", "PORT=13000", "-v", w + ":/app/config", "-p", "127.0.0.1:13000:13000"})
		t.waitFor(60, time.Second, name, "healthcheck never answered up", func() bool {
			if s := t.Docker("inspect", "-f", "{{.State.Status}}", name); s != "running" {
				t.Fatalf("homepage died during startup\n%s", t.Logs(name))
			}
			_, h := Get(b + "/api/healthcheck")
			return h == "up"
		})
		_, svc := Get(b + "/api/services")
		t.Contains(svc, "Quench Group", "services.yaml read")
		t.Contains(svc, "Quench Service", "services.yaml read")
		r, _ := hostReq("GET", "http://127.0.0.1:13000/", "evil.example")
		resp, err := httpc.Do(r)
		if err != nil {
			t.Fatalf("host check: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Fatalf("unlisted Host got %d, expected 400", resp.StatusCode)
		}
		_, page := Get(b + "/")
		t.ContainsFold(page, "<html", "page renders")
		for _, f := range []string{"settings.yaml", "docker.yaml"} {
			if _, err := os.Stat(filepath.Join(w, f)); err != nil {
				t.Fatalf("%s was not copied from the skeleton", f)
			}
		}
		if t.Want != "" {
			if v := t.Docker("exec", name, "/usr/bin/node", "-e", `process.stdout.write(require('/usr/share/homepage/package.json').version)`); v != t.Want {
				t.Fatalf("package version %s, expected %s", v, t.Want)
			}
		}
		if m := regexp.MustCompile(`Read-only file system|EROFS|EACCES|Failed to initialize`).FindString(t.Logs(name)); m != "" {
			t.Fatalf("write errors in logs (%s)\n%s", m, t.Logs(name))
		}
		t.Logf("smoke test passed (nonroot %s, config read, host check live, skeleton copied)", t.User("1001"))
	})
}
