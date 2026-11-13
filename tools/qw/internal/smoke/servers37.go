package smoke

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func init() {
	register("dotnet", func(t *T) {
		if t.Want == "" {
			t.Fatalf("usage: qw smoke dotnet <image-ref> <major>")
		}
		major := strings.SplitN(t.Want, ".", 2)[0]
		ro := []string{"--read-only", "--tmpfs", "/tmp"}
		for _, f := range [][]string{ro, append(append([]string{}, ro...), "--entrypoint", "/usr/bin/dotnet")} {
			if v, _ := t.RunFlags(f, "--version"); !strings.HasPrefix(v, major+".") {
				t.Fatalf("expected '%s.*', got '%s'", major, v)
			}
		}
		info, _ := t.RunFlags(ro, "--info")
		t.Contains(info, ".NET SDK:", "--info SDK section")
		t.Contains(info, "Microsoft.NETCore.App "+major+".", "--info runtime")
		if sdks, _ := t.RunFlags(ro, "--list-sdks"); !strings.HasPrefix(sdks, major+".") {
			t.Fatalf("expected an installed %s.* SDK, got %s", major, sdks)
		}
		t.User("1001")
		t.Contains(t.Docker("inspect", t.Image, "--format", "{{json .Config.Entrypoint}}"), "/usr/bin/dotnet", "entrypoint")
		t.Logf("smoke test passed (.NET %s SDK and runtime, read-only rootfs)", major)
	})
	register("influxdb", func(t *T) {
		const org, bucket, token = "quench", "smoke", "smoke-admin-token-0123456789"
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--tmpfs", "/var/lib/influxdb2:rw,mode=0700,uid=1001,gid=1001",
			"--tmpfs", "/etc/influxdb2:rw,mode=0700,uid=1001,gid=1001", "-e", "DOCKER_INFLUXDB_INIT_MODE=setup", "-e", "DOCKER_INFLUXDB_INIT_USERNAME=admin",
			"-e", "DOCKER_INFLUXDB_INIT_PASSWORD=quenchworks-smoke-pw", "-e", "DOCKER_INFLUXDB_INIT_ORG=" + org, "-e", "DOCKER_INFLUXDB_INIT_BUCKET=" + bucket,
			"-e", "DOCKER_INFLUXDB_INIT_ADMIN_TOKEN=" + token, "-p", "127.0.0.1:8086:8086"})
		t.waitFor(90, 2*time.Second, name, "influxd did not become healthy", func() bool {
			_, h := Get("http://127.0.0.1:8086/health")
			return strings.Contains(h, `"status":"pass"`)
		})
		if c, _ := Get("http://127.0.0.1:8086/ping"); c != 204 {
			t.Fatalf("expected 204 from /ping, got %d", c)
		}
		auth := map[string]string{"Authorization": "Token " + token}
		if c, r := DoH("POST", "http://127.0.0.1:8086/api/v2/write?org="+org+"&bucket="+bucket+"&precision=s",
			"smoke,host=quench value=42 "+strconv.FormatInt(time.Now().Unix(), 10), auth); c/100 != 2 {
			t.Fatalf("write: %d %s", c, r)
		}
		auth["Content-Type"] = "application/vnd.flux"
		_, res := DoH("POST", "http://127.0.0.1:8086/api/v2/query?org="+org, `from(bucket:"`+bucket+`") |> range(start:-1h) |> filter(fn:(r)=>r._measurement=="smoke")`, auth)
		t.Contains(res, ",42", "point returned by query")
		t.Logf("smoke test passed (nonroot user: %s; write+query)", t.User("1001"))
	})
	register("netdata", func(t *T) {
		const b = "http://127.0.0.1:19999/api/v1"
		tm := "rw,uid=1001,gid=1001,mode=0755"
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/var/lib/netdata:" + tm, "--tmpfs", "/var/cache/netdata:" + tm,
			"--tmpfs", "/var/log/netdata:" + tm, "--tmpfs", "/run/netdata:" + tm, "--tmpfs", "/tmp:rw,mode=1777", "-p", "127.0.0.1:19999:19999"})
		info := t.WaitHTTP(b+"/info", 60, name)
		if t.Want != "" {
			t.Match(info, `"version": *"v`+regexp.QuoteMeta(t.Want)+`"`, "version")
		}
		t.waitFor(30, time.Second, name, "system.cpu missing from /api/v1/charts", func() bool { _, c := Get(b + "/charts"); return strings.Contains(c, `"system.cpu"`) })
		t.waitFor(30, time.Second, name, "netdata_system_cpu missing from the Prometheus export", func() bool {
			_, p := Get(b + "/allmetrics?format=prometheus")
			return regexp.MustCompile(`(?m)^netdata_system_cpu`).MatchString(p)
		})
		if _, err := docker("exec", name, "test", "-e", "/usr/share/netdata/web/index.html"); err == nil {
			t.Fatalf("a web dashboard is in the image")
		}
		if strings.Contains(strings.ToLower(t.Logs(name)), "read-only file system") {
			t.Fatalf("read-only rootfs errors\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed (info, charts, Prometheus export, no dashboard; nonroot user: %s)", t.User("1001"))
	})
	register("pinniped", func(t *T) {
		v := lastLine(t.RunEntryAny("/usr/local/bin/pinniped", "version"))
		if v == "" || v == "v0.0.0" || strings.Contains(v, "dev") || (t.Want != "" && v != "v"+t.Want) {
			t.Fatalf("version %q, expected v%s", v, t.Want)
		}
		t.Contains(t.RunAny(), "pinniped-concierge pinniped-supervisor", "multi-call dispatch error lists the roles")
		for _, r := range []string{"pinniped-concierge", "pinniped-supervisor", "local-user-authenticator"} {
			if out := t.RunEntryAny("/usr/local/bin/" + r); strings.Contains(out, "must be invoked as one of") {
				t.Fatalf("%s not dispatched: %s", r, out)
			}
		}
		d := t.TempDir()
		os.Mkdir(filepath.Join(d, "podinfo"), 0o755)
		os.WriteFile(filepath.Join(d, "pinniped.yaml"), []byte(`apiGroupSuffix: pinniped.dev
names:
  servingCertificateSecret: p-api-tls
  credentialIssuer: p-config
  apiService: p-api
  impersonationLoadBalancerService: p-ilb
  impersonationClusterIPService: p-icip
  impersonationTLSCertificateSecret: p-itls
  impersonationCACertificateSecret: p-ica
  impersonationSignerSecret: p-isig
  agentServiceAccount: p-agent
  impersonationProxyServiceAccount: p-ipsa
  impersonationProxyLegacySecret: p-ipls
labels: {"app":"p"}
kubeCertAgent:
  namePrefix: p-kube-cert-agent-
  image: example/image@sha256:0000000000000000000000000000000000000000000000000000000000000000
`), 0o644)
		for f, c := range map[string]string{"labels": "app=\"p\"\n", "namespace": "pinniped", "name": "p-0", "uid": "uid"} {
			os.WriteFile(filepath.Join(d, "podinfo", f), []byte(c), 0o644)
		}
		out, _ := t.RunFlags([]string{"-v", d + ":/etc/p:ro", "--entrypoint", "/usr/local/bin/pinniped-concierge"}, "--config", "/etc/p/pinniped.yaml", "--downward-api-path", "/etc/p/podinfo")
		t.Contains(out, "could not load in-cluster configuration", "concierge got past its config")
		t.Logf("smoke test passed (version %s, multi-call dispatch, concierge config load; user %s)", v, t.Docker("inspect", t.Image, "--format", "{{.Config.User}}"))
	})
}
