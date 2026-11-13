package smoke

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// insecure is an HTTP client for self-signed TLS endpoints.
var insecure = &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}

func insecureDo(req *http.Request) (int, string) {
	r, err := insecure.Do(req)
	if err != nil {
		return 0, err.Error()
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return r.StatusCode, string(b)
}

func init() {
	register("vllm", func(t *T) {
		t.User("1001")
		v := lastLine(t.RunEntryAny("/usr/bin/vllm", "--version"))
		t.Match(v, `^[0-9]+\.[0-9]+\.[0-9]+\+cpu$`, "<X.Y.Z>+cpu version")
		if t.Want != "" && v != t.Want+"+cpu" {
			t.Fatalf("expected %s+cpu, got %s", t.Want, v)
		}
		imp := lastLine(t.RunEntryAny("/opt/vllm/venv/bin/python", "-c", `import vllm; print("IMPORT_OK", vllm.__version__)`))
		t.Match(imp, `^IMPORT_OK `, "import vllm")
		t.Logf("smoke test passed (vllm %s, import vllm)", v)
	})
	register("zot", func(t *T) {
		name, _ := t.httpUp([]string{"-p", "127.0.0.1:5000:5000"}, "http://127.0.0.1:5000/v2/", 30)
		out, _ := docker("exec", name, "/usr/bin/zot", "--version")
		t.Match(out, `(?i)commit[^0-9]*v?[0-9]+\.[0-9]+\.[0-9]+`, "version stamped")
		t.Logf("smoke test passed (nonroot user: %s)", t.User("1001"))
	})
	register("mattermost", func(t *T) {
		t.User("1001")
		if wd := t.Docker("inspect", "--format", "{{.Config.WorkingDir}}", t.Image); wd != "/opt/mattermost" {
			t.Fatalf("work-dir is %q, not /opt/mattermost", wd)
		}
		out, err := t.RunFlags([]string{"--read-only", "--entrypoint", "mattermost"}, "version")
		if err != nil {
			t.Fatalf("version: %s", out)
		}
		t.Match(out, `(?m)^Version: `, "Version line")
		t.Contains(out, "Build Enterprise Ready: false", "Team Edition")
		if t.Want != "" {
			t.Match(out, `(?m)^Version: `+regexp.QuoteMeta(t.Want)+`$`, "version "+t.Want)
		}
		t.Logf("smoke test passed")
	})
	register("ntfy", func(t *T) {
		t.Match(t.RunEntryAny("/usr/bin/ntfy", "--version"), `(?i)v?[0-9]+\.[0-9]+\.[0-9]+`, "version stamped")
		u := t.User("1001")
		t.httpUp([]string{"-p", "18080:8080"}, "http://127.0.0.1:18080/v1/health", 30)
		if c, _ := Get("http://127.0.0.1:18080/"); c != 200 {
			t.Fatalf("web app not served at / (HTTP %d)", c)
		}
		t.Logf("smoke test passed (nonroot user: %s, /v1/health + web UI)", u)
	})
	register("openfga", func(t *T) {
		t.Match(t.RunEntryAny("/usr/bin/openfga", "version"), `(?i)version .v?[0-9]+\.[0-9]+\.[0-9]+`, "version stamped")
		_, h := t.httpUp([]string{"-p", "127.0.0.1:8080:8080"}, "http://127.0.0.1:8080/healthz", 30)
		t.ContainsFold(h, "SERVING", "healthz")
		t.Logf("smoke test passed (nonroot user: %s)", t.User("1001"))
	})
	register("pyroscope", func(t *T) {
		t.Match(t.RunEntryAny("/usr/bin/pyroscope", "-version"), `(?i)pyroscope, version\s+[0-9]+\.[0-9]+\.[0-9]+`, "version stamped")
		t.httpUp([]string{"-p", "127.0.0.1:4040:4040"}, "http://127.0.0.1:4040/ready", 150)
		t.Logf("smoke test passed (nonroot user: %s)", t.User("1001"))
	})
	register("renovate", func(t *T) {
		t.Match(t.RunAny("--version"), `(?m)^[0-9]+\.[0-9]+\.[0-9]+`, "semver")
		u := t.User("1001")
		fix := t.TempDir()
		os.MkdirAll(filepath.Join(fix, "sub"), 0o755)
		os.WriteFile(filepath.Join(fix, "package.json"), []byte(`{"name":"x","dependencies":{"left-pad":"1.0.0"}}`+"\n"), 0o644)
		os.WriteFile(filepath.Join(fix, "sub/Dockerfile"), []byte("FROM node:20.1.0\n"), 0o644)
		os.WriteFile(filepath.Join(fix, "renovate.json"), []byte(`{"extends":[],"enabledManagers":["npm","dockerfile"]}`+"\n"), 0o644)
		log, _ := t.RunFlags([]string{"-v", fix + ":/repo:ro", "-w", "/repo", "-e", "LOG_LEVEL=debug", "-e", "RENOVATE_PLATFORM=local",
			"-e", "RENOVATE_DRY_RUN=extract", "-e", "HOME=/tmp", "-e", "RENOVATE_BASE_DIR=/tmp/renovate"})
		for _, w := range []string{`"depName": "left-pad"`, `"depName": "node"`, "using glob instead"} {
			t.Contains(log, w, "renovate extract")
		}
		t.Logf("smoke test passed (extract found left-pad + node; nonroot user: %s)", u)
	})
	register("cadence", func(t *T) {
		rv := `(?i)Release version:\s*[0-9]+\.[0-9]+\.[0-9]+`
		t.Match(t.RunAny("--version"), rv, "server version stamped")
		t.Match(t.RunEntryAny("/usr/bin/cadence", "--version"), rv, "cli version stamped")
		tool := t.RunEntryAny("/usr/bin/cadence-cassandra-tool", "--help")
		for _, w := range []string{"cadence cassandra operations", "setup-schema", "update-schema"} {
			t.ContainsFold(tool, w, "cassandra-tool")
		}
		t.Logf("smoke test passed (nonroot user: %s)", t.User("1001"))
	})
	register("falco", func(t *T) {
		v := t.RunAny("--version")
		t.Contains(v, "Falco version:", "--version")
		if t.Want != "" {
			t.Contains(v, "Falco version: "+t.Want, "--version")
		}
		p := t.RunAny("--list-plugins")
		t.Contains(p, "Name: container", "--list-plugins")
		t.Contains(p, "Version: 0.7.6", "--list-plugins")
		t.Contains(t.RunAny("-L"), "Terminal shell in container", "-L")
		t.Contains(t.RunAny("-V", "/etc/falco/falco_rules.yaml"), ": Ok", "-V")
		l := t.RunAny("--list")
		t.Contains(l, "container.id", "--list")
		t.Contains(l, "proc.name", "--list")
		t.Logf("smoke test passed (version, container plugin, default rules load and validate, fields)")
	})
	register("gitlab-runner", func(t *T) {
		u := t.User("1001")
		v := t.RunEntry("/usr/bin/gitlab-runner", "--version")
		if t.Want != "" {
			t.Match(v, `(?m)^Version: +`+regexp.QuoteMeta(t.Want)+`$`, "version")
		}
		cfg := filepath.Join(t.TempDir(), "config.toml")
		os.WriteFile(cfg, []byte("concurrent = 1\nlisten_address = \":9252\"\n"), 0o644)
		name := t.Name("")
		t.Start(name, []string{"-p", "127.0.0.1:9252:9252", "-v", cfg + ":/home/gitlab-runner/config.toml:ro"})
		var m string
		for i := 0; ; i++ {
			if _, m = Get("http://127.0.0.1:9252/metrics"); regexp.MustCompile(`(?m)^gitlab_runner_version_info`).MatchString(m) {
				break
			}
			if i == 30 {
				t.Fatalf("no metrics from gitlab-runner run\n%s", t.Logs(name))
			}
			time.Sleep(time.Second)
		}
		if t.Want != "" {
			t.Contains(m, `version="`+t.Want+`"`, "metrics version")
		}
		t.Logf("smoke test passed (run loop up, metrics served; nonroot user: %s)", u)
	})
	register("linkerd-policy-controller", func(t *T) {
		name := t.Name("")
		t.Start(name, []string{"-p", "127.0.0.1:9990:9990", "-p", "127.0.0.1:8090:8090"},
			"--default-opaque-ports=25,587,3306,4444,5432,6379,9300,11211", "--admission-controller-disabled")
		time.Sleep(8 * time.Second)
		logs := t.Logs(name)
		t.Contains(logs, "linkerd_policy_controller_runtime", "reached its own runtime")
		t.Contains(logs, "Connection refused", "failed on the unreachable API server")
		if m := regexp.MustCompile(`panic|SIGSEGV|no such file or directory|cannot execute|error while loading shared`).FindString(logs); m != "" {
			t.Fatalf("the binary did not start cleanly (%s):\n%s", m, logs)
		}
		t.Logf("boot test passed (nonroot user: %s)", t.User("1001"))
	})
	register("nifi", func(t *T) {
		u := t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"-p", "127.0.0.1:8443:8443", "--entrypoint", "/bin/sh"}, "-c",
			"/opt/nifi/bin/nifi.sh set-single-user-credentials admin quench-smoke-pass && exec /opt/nifi/bin/nifi.sh run")
		base := "https://localhost:8443"
		for i := 0; ; i++ {
			req, _ := http.NewRequest("GET", base+"/nifi/", nil)
			if c, _ := insecureDo(req); c == 200 {
				break
			}
			if i == 90 {
				t.Fatalf("nifi did not come up\n%s", t.Logs(name))
			}
			time.Sleep(4 * time.Second)
		}
		req, _ := http.NewRequest("POST", base+"/nifi-api/access/token", strings.NewReader(url.Values{"username": {"admin"}, "password": {"quench-smoke-pass"}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		c, tok := insecureDo(req)
		if c/100 != 2 || tok == "" {
			t.Fatalf("login failed: %d %s", c, tok)
		}
		get := func(p string) string {
			req, _ := http.NewRequest("GET", base+p, nil)
			req.Header.Set("Authorization", "Bearer "+tok)
			c, b := insecureDo(req)
			if c != 200 {
				t.Fatalf("%s: %d %s", p, c, b)
			}
			return b
		}
		about := get("/nifi-api/flow/about")
		if t.Want != "" {
			t.Contains(about, `"version":"`+t.Want+`"`, "version")
		}
		t.Contains(get("/nifi-api/flow/processor-types?type=org.apache.nifi.processors.aws.s3.PutS3Object"), `"artifact":"nifi-aws-nar"`, "aws NAR loaded")
		t.Logf("smoke test passed (nifi: UI, login, aws NAR; nonroot user: %s)", u)
	})
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	return s[strings.LastIndex(s, "\n")+1:]
}
