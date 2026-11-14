package smoke

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// kgatewayEnvoyBootstrap is a static listener whose direct response is the Downward API
// template envoyinit expands.
const kgatewayEnvoyBootstrap = `admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 19000 }
static_resources:
  listeners:
    - name: smoke
      address:
        socket_address: { address: 0.0.0.0, port_value: 8080 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                stat_prefix: smoke
                route_config:
                  name: smoke
                  virtual_hosts:
                    - name: smoke
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/" }
                          direct_response:
                            status: 200
                            body: { inline_string: "{{.PodName}}" }
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
`

func init() {
	// web on a read-only rootfs against Postgres: /api/v1/info reports the version, the Elm UI
	// serves, a local user logs in and sees main; a privileged containerd worker registers over
	// the TSA, advertises registry-image and time, and runs a time check in a worker container.
	register("concourse", func(t *T) {
		const port = "18081"
		const b = "http://127.0.0.1:" + port
		u := t.User("1001")
		if v := t.Run("--version"); t.Want != "" && v != t.Want {
			t.Fatalf("version %q, expected %s", v, t.Want)
		}
		keys := t.TempDir()
		for _, k := range [][]string{{"rsa", "session_signing_key"}, {"ssh", "tsa_host_key"}, {"ssh", "worker_key"}} {
			if out, err := t.RunFlags([]string{"-v", keys + ":/keys"}, "generate-key", "-t", k[0], "-f", "/keys/"+k[1]); err != nil {
				t.Fatalf("generate-key %s: %s", k[1], out)
			}
		}
		filepath.Walk(keys, func(p string, _ os.FileInfo, _ error) error { os.Chmod(p, 0o755); return nil })

		net_, pg, web, worker := t.Name("net"), t.Name("pg"), t.Name(""), t.Name("worker")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Cleanup(func() { docker("rm", "-f", pg, worker) })
		t.Docker("run", "-d", "--name", pg, "--network", net_, "-e", "POSTGRES_USER=concourse", "-e", "POSTGRES_PASSWORD=pw", "-e", "POSTGRES_DB=concourse",
			envOr("PG_IMAGE", "postgres:17"))
		t.waitFor(60, time.Second, pg, "postgres never accepted a query", func() bool {
			_, err := docker("exec", pg, "psql", "-U", "concourse", "-d", "concourse", "-c", "select 1")
			return err == nil
		})
		t.Start(web, []string{"--network", net_, "--read-only", "--tmpfs", "/tmp", "-v", keys + ":/keys:ro", "-p", "127.0.0.1:" + port + ":8080",
			"-e", "CONCOURSE_POSTGRES_HOST=" + pg, "-e", "CONCOURSE_POSTGRES_USER=concourse", "-e", "CONCOURSE_POSTGRES_PASSWORD=pw",
			"-e", "CONCOURSE_POSTGRES_DATABASE=concourse", "-e", "CONCOURSE_EXTERNAL_URL=" + b,
			"-e", "CONCOURSE_ADD_LOCAL_USER=smoke:Smoke-Pass-2026", "-e", "CONCOURSE_MAIN_TEAM_LOCAL_USER=smoke",
			"-e", "CONCOURSE_SESSION_SIGNING_KEY=/keys/session_signing_key", "-e", "CONCOURSE_TSA_HOST_KEY=/keys/tsa_host_key",
			"-e", "CONCOURSE_TSA_AUTHORIZED_KEYS=/keys/worker_key.pub"}, "web")
		var info string
		t.waitFor(90, time.Second, web, "web never answered /api/v1/info", func() bool {
			c, body := Get(b + "/api/v1/info")
			info = body
			return c == 200
		})
		if t.Want != "" {
			t.Contains(info, `"version":"`+t.Want+`"`, "/api/v1/info version")
		}
		_, page := Get(b + "/")
		t.Contains(page, "elm.min.js", "embedded UI")
		if c, _ := Get(b + "/public/elm.min.js"); c != 200 {
			t.Fatalf("the compiled UI bundle answered %d", c)
		}
		_, tok := DoH("POST", b+"/sky/issuer/token", "grant_type=password&username=smoke&password=Smoke-Pass-2026&scope=openid+profile+email+federated:id+groups",
			map[string]string{"Authorization": "Basic Zmx5OlpteDU=", "Content-Type": "application/x-www-form-urlencoded"})
		var at struct {
			Token string `json:"access_token"`
		}
		if json.Unmarshal([]byte(tok), &at) != nil || at.Token == "" {
			t.Fatalf("password login did not return a token: %s", tok)
		}
		auth := map[string]string{"Authorization": "Bearer " + at.Token}
		_, teams := DoH("GET", b+"/api/v1/teams", "", auth)
		t.Contains(teams, `"name":"main"`, "main team visible to its local user")

		t.Docker("run", "-d", "--name", worker, "--network", net_, "--privileged", "--user", "0", "--tmpfs", "/tmp", "-v", keys+":/keys:ro",
			"-e", "CONCOURSE_TSA_HOST="+web+":2222", "-e", "CONCOURSE_TSA_PUBLIC_KEY=/keys/tsa_host_key.pub",
			"-e", "CONCOURSE_TSA_WORKER_PRIVATE_KEY=/keys/worker_key", "-e", "CONCOURSE_RUNTIME=containerd",
			"-e", "CONCOURSE_WORK_DIR=/tmp/worker", "-e", "CONCOURSE_CONTAINERD_DNS_SERVER=8.8.8.8", t.Image, "worker")
		var workers string
		t.waitFor(90, 2*time.Second, worker, "the worker never reached running", func() bool {
			_, workers = DoH("GET", b+"/api/v1/workers", "", auth)
			return strings.Contains(workers, `"state":"running"`)
		})
		for _, rt := range []string{"registry-image", "time"} {
			t.Contains(workers, `"type":"`+rt+`"`, "worker resource type "+rt)
		}
		pipe := "resources: [{name: tick, type: time, source: {interval: 1m}}]\njobs: [{name: noop, plan: [{get: tick}]}]"
		if c, body := DoH("PUT", b+"/api/v1/teams/main/pipelines/smoke/config", pipe, map[string]string{"Authorization": auth["Authorization"], "Content-Type": "application/x-yaml"}); c >= 300 {
			t.Fatalf("setting the smoke pipeline: %d %s", c, body)
		}
		DoH("PUT", b+"/api/v1/teams/main/pipelines/smoke/unpause", "", auth)
		_, chk := DoH("POST", b+"/api/v1/teams/main/pipelines/smoke/resources/tick/check", "{}", map[string]string{"Authorization": auth["Authorization"], "Content-Type": "application/json"})
		var build struct {
			ID     int    `json:"id"`
			Status string `json:"status"`
		}
		if json.Unmarshal([]byte(chk), &build) != nil || build.ID == 0 {
			t.Fatalf("the check did not start: %s", chk)
		}
		t.waitFor(60, 2*time.Second, worker, "the time check never finished", func() bool {
			_, body := DoH("GET", b+"/api/v1/builds/"+strconv.Itoa(build.ID), "", auth)
			json.Unmarshal([]byte(body), &build)
			return build.Status != "" && build.Status != "started" && build.Status != "pending"
		})
		if build.Status != "succeeded" {
			t.Fatalf("the time check ended %s\n%s", build.Status, t.Logs(worker))
		}
		t.Logf("smoke test passed (concourse %s: API, UI, local login, worker running, a time check ran in a worker container; user %s)", t.Want, u)
	})

	// The entrypoint runs envoyinit, which expands the Downward API template in the bootstrap
	// before execing envoy; the bundled envoy is the minor this kgateway line expects, and the
	// preStop drain hook's shell and wget are present.
	register("kgateway-envoy", func(t *T) {
		u := t.User("1001")
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "envoy.yaml"), []byte(kgatewayEnvoyBootstrap), 0o644)
		name := t.Name("")
		// a bind mount of a temp dir is refused under Docker Desktop: docker cp before start
		t.Cleanup(func() { docker("rm", "-f", name) })
		t.Docker("create", "--name", name, "-e", "POD_NAME=quench-smoke-pod", "-e", "POD_NAMESPACE=default", "-p", "127.0.0.1:18082:8080", t.Image, "--log-level", "warn")
		t.Docker("cp", filepath.Join(dir, "envoy.yaml"), name+":/etc/envoy/envoy.yaml")
		t.Docker("start", name)
		var body string
		t.waitFor(30, time.Second, name, "envoy never served the smoke listener", func() bool {
			c, b := Get("http://127.0.0.1:18082/")
			body = b
			return c == 200
		})
		if body != "quench-smoke-pod" {
			t.Fatalf("expected {{.PodName}} to expand to quench-smoke-pod, got %q", body)
		}
		ev := t.RunEntry("/usr/local/bin/envoy", "--version")
		minor := t.Match(ev, `/(\d+\.\d+)\.\d+/`, "envoy version")
		line := regexp.MustCompile(`^\d+\.\d+`).FindString(t.Want)
		if want := map[string]string{"2.3": "1.37", "2.4": "1.38"}[line]; want != "" && minor != want {
			t.Fatalf("kgateway %s expects envoy %s.x, image has %s", t.Want, want, ev)
		}
		if out := t.RunEntryAny("/bin/sh", "-c", "command -v wget >/dev/null && echo ok"); out != "ok" {
			t.Fatalf("/bin/sh or wget missing; the preStop drain hook would fail: %q", out)
		}
		t.Logf("smoke test passed (kgateway-envoy %s: envoyinit expands the Downward API, envoy %s, drain hook tools; user %s)", t.Want, minor, u)
	})
}
