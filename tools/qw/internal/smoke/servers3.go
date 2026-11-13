package smoke

import (
	"regexp"
	"strings"
	"time"
)

// kv: a Redis-protocol server on a read-only root answers PING, then SET/GET through cli.
func kv(cli string) func(*T) {
	return func(t *T) {
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp"})
		for i := 0; ; i++ {
			if out, _ := docker("exec", name, cli, "ping"); strings.Contains(out, "PONG") {
				break
			}
			if i == 30 {
				t.Fatalf("%s did not become ready\n%s", t.App, t.Logs(name))
			}
			time.Sleep(time.Second)
		}
		t.Docker("exec", name, cli, "set", "qw", "hello")
		if v := t.Docker("exec", name, cli, "get", "qw"); v != "hello" {
			t.Fatalf("SET/GET failed: %s", v)
		}
		u := t.User("1001")
		t.Logf("smoke test passed (PING, SET/GET, nonroot user: %s)", u)
	}
}

func init() {
	register("redis", kv("redis-cli"))
	register("valkey", kv("valkey-cli"))
	register("dagster", func(t *T) {
		if t.Want == "" {
			t.Fatalf("usage: qw smoke dagster <image-ref> <version>")
		}
		t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp", "--tmpfs", "/var/lib/dagster:uid=1001,gid=1001", "-p", "127.0.0.1:13000:3000"})
		info := t.WaitHTTP("http://127.0.0.1:13000/server_info", 90, name)
		t.Logf("%s", info)
		t.Match(info, `"dagster_version": *"`+regexp.QuoteMeta(t.Want)+`"`, "server_info version")
		_, gql := DoH("POST", "http://127.0.0.1:13000/graphql", `{"query":"{ version }"}`, map[string]string{"Content-Type": "application/json"})
		t.Contains(gql, t.Want, "graphql version")
		t.RunEntry("/opt/dagster/venv/bin/dagster-daemon", "--help")
		t.Logf("smoke test passed")
	})
	register("filebrowser", func(t *T) {
		out := t.Run("version")
		t.Logf("%s", out)
		t.Match(out, `(?i)v?[0-9]+\.[0-9]+\.[0-9]+`, "version not stamped")
		t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"-p", "18080:8080"})
		t.WaitHTTP("http://127.0.0.1:18080/", 30, name)
		t.Logf("smoke test passed")
	})
	register("kuberay", func(t *T) {
		t.User("1001")
		kc := t.DeadKubeconfig()
		out, _ := t.RunAnyFor(60*time.Second, []string{"--read-only", "-v", kc + ":/w/kubeconfig:ro", "-e", "KUBECONFIG=/w/kubeconfig"})
		t.Contains(out, `"msg":"Loaded feature gates"`, "feature gates loaded")
		t.Contains(out, `"msg":"Setup manager"`, "manager setup")
		t.Contains(out, "127.0.0.1:1", "reached the API server step")
		t.Logf("smoke test passed")
	})
	register("opentelemetry-operator", func(t *T) {
		t.User("1001")
		kc := t.DeadKubeconfig()
		out, _ := t.RunAnyFor(60*time.Second, []string{"--read-only", "--tmpfs", "/tmp", "-v", kc + ":/etc/kubeconfig:ro",
			"-e", "KUBECONFIG=/etc/kubeconfig", "-e", "NAMESPACE=opentelemetry-operator-system"})
		start := regexp.MustCompile(`.*Starting the OpenTelemetry Operator.*`).FindString(out)
		if start == "" {
			t.Fatalf("no start line:\n%s", out)
		}
		if t.Want != "" {
			t.Contains(start, `"opentelemetry-operator":"`+t.Want+`"`, "stamped version")
		}
		t.Match(start, `"collector-image":"ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector:0\.[1-9]`, "default component versions")
		t.Contains(out, "127.0.0.1:1", "dialed the configured API server")
		noCrash(t, out)
		t.Logf("smoke test passed")
	})
	register("mastodon-streaming", func(t *T) {
		t.User("1001")
		net, pg, rd, name := t.Name("net"), t.Name("pg"), t.Name("redis"), t.Name("")
		t.Docker("network", "create", net)
		t.Cleanup(func() { docker("network", "rm", net) })
		t.Cleanup(func() { docker("rm", "-f", pg, rd) })
		t.Docker("run", "-d", "--name", pg, "--network", net, "-e", "POSTGRES_USER=mastodon", "-e", "POSTGRES_PASSWORD=pw", "-e", "POSTGRES_DB=mastodon_production", "postgres:17")
		t.Docker("run", "-d", "--name", rd, "--network", net, "redis:7")
		for i := 0; i < 60; i++ {
			if _, err := docker("exec", pg, "psql", "-U", "mastodon", "-d", "mastodon_production", "-c", "select 1"); err == nil {
				break
			}
			time.Sleep(time.Second)
		}
		t.Start(name, []string{"--network", net, "--read-only", "--tmpfs", "/tmp", "-e", "DB_HOST=" + pg, "-e", "DB_USER=mastodon", "-e", "DB_PASS=pw",
			"-e", "DB_NAME=mastodon_production", "-e", "REDIS_HOST=" + rd, "-p", "127.0.0.1:14000:4000"})
		for i := 0; ; i++ {
			if _, b := Get("http://127.0.0.1:14000/api/v1/streaming/health"); b == "OK" {
				break
			}
			if i == 60 {
				t.Fatalf("streaming never answered /api/v1/streaming/health\n%s", t.Logs(name))
			}
			time.Sleep(time.Second)
		}
		if code, _ := Get("http://127.0.0.1:14000/api/v1/streaming/public"); code != 401 {
			t.Fatalf("expected 401 without a token, got %d\n%s", code, t.Logs(name))
		}
		if m := regexp.MustCompile(`(?i)"level":(50|60)|Error: Cannot find module`).FindString(t.Logs(name)); m != "" {
			t.Fatalf("streaming logged errors: %s", m)
		}
		t.Logf("smoke test passed")
	})
	register("vitess", func(t *T) {
		if t.Want == "" {
			t.Fatalf("usage: qw smoke vitess <image-ref> <version>")
		}
		t.User("1001")
		for _, b := range []string{"vtgate", "vttablet", "vtctld", "vtctldclient", "vtorc", "mysqlctl", "mysqlctld", "vtbackup"} {
			t.RunEntry("/usr/bin/"+b, "--version")
		}
		t.Contains(t.RunEntry("/usr/bin/vtgate", "--version"), "Version: "+t.Want+" ", "vtgate version")
		t.RunEntry("/usr/bin/vtctldclient", "--help")
		t.Logf("smoke test passed")
	})
	register("actions-runner", func(t *T) {
		t.User("1001")
		v := strings.ReplaceAll(t.RunEntry("/home/runner/bin/Runner.Listener", "--version"), "\r", "")
		t.Logf("runner version: %s", v)
		if v == "" || (t.Want != "" && v != t.Want) {
			t.Fatalf("runner version '%s', expected %s", v, t.Want)
		}
		for _, n := range []string{"node24", "node20"} {
			nv := t.RunEntry("/home/runner/externals/"+n+"/bin/node", "-e", "process.stdout.write(process.version)")
			t.Logf("externals/%s: node %s", n, nv)
			if !strings.HasPrefix(nv, "v24.") {
				t.Fatalf("externals/%s does not run Node 24", n)
			}
		}
		t.RunEntry("/usr/bin/git", "--version")
		t.Contains(t.RunEntry("/usr/bin/tar", "--version"), "GNU tar", "GNU tar")
		t.Contains(t.RunAny(), "set RUNNER_URL", "entrypoint explains the missing registration")
		t.Logf("smoke test passed")
	})
	register("longhorn-manager", func(t *T) {
		// root, like upstream's chart; no ENTRYPOINT, because the chart passes only args
		if u := t.Docker("inspect", t.Image, "--format", "{{.Config.User}}"); u != "0" && u != "" {
			t.Fatalf("expected root (upstream's chart sets no user), got '%s'", u)
		}
		if ep := t.Docker("inspect", t.Image, "--format", "{{json .Config.Entrypoint}}"); ep != "null" && ep != "[]" {
			t.Fatalf("image has an ENTRYPOINT (%s); args-only containers would break", ep)
		}
		t.WantVersionV(t.Run("longhorn-manager", "--version"))
		t.RunEntry("bash", "-c", `for t in launch-manager nsmounter nsenter mount umount mkfs.ext4 mkfs.xfs mount.nfs; do command -v $t >/dev/null || { echo "missing $t"; exit 1; }; done`)
		img := "example.invalid/x:1"
		out, _ := t.RunFlags([]string{"-e", "NODE_NAME=smoke", "--entrypoint", "longhorn-manager"}, "-d", "daemon",
			"--engine-image", img, "--instance-manager-image", img, "--share-manager-image", img,
			"--backing-image-manager-image", img, "--support-bundle-manager-image", img,
			"--manager-image", img, "--service-account", "longhorn")
		t.Contains(out, "failed to check environment", "daemon reached the environment check")
		t.Logf("smoke test passed")
	})
}
