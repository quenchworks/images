package smoke

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func init() {
	register("linkerd-viz-metrics-api", func(t *T) {
		// a TLS fake of the two API calls metrics-api makes at boot; everything else 404s
		srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch r.Method + " " + r.URL.Path {
			case "POST /apis/authorization.k8s.io/v1/selfsubjectaccessreviews":
				w.WriteHeader(201)
				io.WriteString(w, `{"apiVersion":"authorization.k8s.io/v1","kind":"SelfSubjectAccessReview","status":{"allowed":true}}`)
			case "GET /apis/linkerd.io/v1alpha2":
				io.WriteString(w, `{"kind":"APIResourceList","apiVersion":"v1","groupVersion":"linkerd.io/v1alpha2","resources":[{"name":"serviceprofiles","namespaced":true,"kind":"ServiceProfile","verbs":["list","watch"]}]}`)
			default:
				w.WriteHeader(404)
				io.WriteString(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"NotFound","code":404}`)
			}
		}))
		ln, err := net.Listen("tcp", "0.0.0.0:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		srv.Listener = ln
		srv.StartTLS()
		t.Cleanup(srv.Close)
		d := t.TempDir()
		os.WriteFile(filepath.Join(d, "kubeconfig"), []byte(`apiVersion: v1
kind: Config
clusters: [{name: s, cluster: {server: "https://host.docker.internal:`+strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)+`", insecure-skip-tls-verify: true}}]
users: [{name: s, user: {token: standalone-test-token}}]
contexts: [{name: s, context: {cluster: s, user: s}}]
current-context: s
`), 0o644)
		name := t.Name("")
		t.Start(name, append([]string{"-p", "127.0.0.1:9995:9995", "-v", d + ":/cfg:ro"}, hostStubFlag...), "-kubeconfig=/cfg/kubeconfig")
		t.waitFor(30, time.Second, name, "admin server (:9995/ping) did not answer", func() bool { _, p := Get("http://127.0.0.1:9995/ping"); return strings.TrimSpace(p) == "pong" })
		_, m := Get("http://127.0.0.1:9995/metrics")
		t.Match(m, `(?m)^# TYPE go_goroutines`, "Prometheus exposition")
		if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
			t.Fatalf("process exited\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed (admin server up against a fake API server)")
	})
	register("mediawiki", func(t *T) {
		const b = "http://127.0.0.1:8099"
		u := t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp", "--tmpfs", "/var/www/html/images:mode=1777", "-e", "MW_CONFIG_FILE=/tmp/mw/LocalSettings.php", "-p", "127.0.0.1:8099:8080"})
		t.waitFor(30, time.Second, name, "nginx never answered", func() bool { c, _ := Get(b + "/index.php"); return c != 0 })
		if out, err := docker("exec", name, "sh", "-c", `mkdir -p /tmp/mw /tmp/mwdata && cd /var/www/html && \
php maintenance/run.php install --confpath=/tmp/mw --dbtype=sqlite --dbpath=/tmp/mwdata \
  --server=http://127.0.0.1:8099 --scriptpath="" --pass=Quench-Smoke-Pass-2026 QuenchSmoke admin`); err != nil {
			t.Fatalf("maintenance/run.php install failed:\n%s", out)
		}
		t.Docker("exec", name, "test", "-f", "/tmp/mw/LocalSettings.php")
		if out, err := docker("exec", "-e", "MW_CONFIG_FILE=/tmp/mw/LocalSettings.php", name, "sh", "-c", "cd /var/www/html && php maintenance/run.php update --quick"); err != nil {
			t.Fatalf("maintenance/run.php update failed:\n%s", out)
		}
		c, page := Get(b + "/index.php?title=Main_Page")
		if c != 200 || !strings.Contains(page, "MediaWiki has been installed") {
			t.Fatalf("Main_Page: HTTP %d\n%.600s", c, page)
		}
		_, si := Get(b + "/api.php?action=query&meta=siteinfo&format=json")
		m := regexp.MustCompile(`"generator":"(MediaWiki [^"]*)"`).FindStringSubmatch(si)
		if m == nil || (t.Want != "" && m[1] != "MediaWiki "+t.Want) {
			t.Fatalf("api generator %v, expected MediaWiki %s", m, t.Want)
		}
		if c, _ := Get(b + "/maintenance/run.php"); c != 403 {
			t.Fatalf("maintenance/ is reachable over HTTP (got %d)", c)
		}
		t.Logf("smoke test passed (%s, SQLite install, Main_Page 200, maintenance/ 403, nonroot user: %s)", m[1], u)
	})
	register("nginx-gateway-fabric-nginx", func(t *T) {
		nv := regexp.MustCompile(`nginx/([0-9.]+)`).FindStringSubmatch(t.RunEntryAny("/usr/bin/nginx", "-v"))
		if nv == nil {
			t.Fatalf("nginx version not reported")
		}
		conf := t.RunEntryAny("/usr/bin/nginx", "-V")
		for _, f := range []string{"--with-stream_ssl_preread_module", "--with-http_ssl_module", "--with-http_v2_module", "--with-compat", "--add-dynamic-module=../njs"} {
			t.Contains(conf, f, "nginx build flags")
		}
		t.Match(t.RunEntryAny("/usr/bin/nginx-agent", "--version"), `[0-9]+\.[0-9]+\.[0-9]+`, "nginx-agent version stamped")
		if out, err := t.RunFlags([]string{"--entrypoint", "/usr/bin/nginx"}, "-t", "-c", "/etc/nginx/nginx.conf"); err != nil {
			t.Fatalf("NGF nginx.conf failed to parse:\n%s", out)
		}
		t.RunEntry("/bin/sh", "-c", "test -f /etc/nginx/grpc-error-locations.conf && test -f /etc/nginx/grpc-error-pages.conf")
		name := t.Name("")
		t.Start(name, []string{"--entrypoint", "/bin/sh"}, "-c", `printf "log:\n  level: info\n  path: /var/log/nginx-agent/\nallowed_directories:\n  - /etc/nginx\n  - /var/run/nginx\n" > /etc/nginx-agent/nginx-agent.conf
exec /usr/bin/docker-entrypoint.sh`)
		time.Sleep(10 * time.Second)
		if s := t.Docker("inspect", name, "--format", "{{.State.Status}}"); s != "running" {
			t.Fatalf("container is %s, the supervisor died\n%s", s, t.Logs(name))
		}
		procs, err := docker("top", name, "-eo", "args")
		if err != nil { // some daemons reject ps options
			procs = t.Docker("top", name)
		}
		t.Contains(procs, "/usr/bin/nginx -g", "nginx master under the entrypoint")
		t.Contains(procs, "/usr/bin/nginx-agent", "nginx-agent under the entrypoint")
		t.Docker("exec", name, "test", "-f", "/var/run/nginx/nginx.pid")
		t.Logf("smoke test passed (nginx %s, agent stamped, NGF nginx.conf parses, user %s)", nv[1], t.User("101"))
	})
	register("notation", func(t *T) {
		const oras = "ghcr.io/quenchworks/images/oras:1.3.4"
		u := t.User("1001")
		if v := t.RunAny("version"); t.Want != "" {
			t.Match(v, `Version: *v`+regexp.QuoteMeta(t.Want), "version")
		}
		net_, reg := t.Name("net"), t.Name("reg")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Cleanup(func() { docker("rm", "-f", reg) })
		t.Docker("run", "-d", "--name", reg, "--network", net_, "--tmpfs", "/var/lib/registry:uid=1001,gid=1001", "ghcr.io/quenchworks/images/distribution:3.1.2")
		w := t.TempDir()
		home, data := filepath.Join(w, "home"), filepath.Join(w, "data")
		for _, d := range []string{home, data} {
			os.Mkdir(d, 0o777)
			os.Chmod(d, 0o777)
		}
		os.WriteFile(filepath.Join(data, "a.txt"), []byte("signed payload\n"), 0o644)
		os.WriteFile(filepath.Join(data, "b.txt"), []byte("unsigned payload\n"), 0o644)
		or := func(args ...string) (string, error) {
			return docker(append([]string{"run", "--rm", "--network", net_, "--read-only", "--tmpfs", "/tmp", "--tmpfs", "/home/nonroot:uid=1001,gid=1001",
				"-v", data + ":/work", "-w", "/work", oras}, args...)...)
		}
		nt := func(args ...string) (string, error) {
			return t.RunFlags([]string{"--network", net_, "--read-only", "--tmpfs", "/tmp", "-v", home + ":/home/nonroot"}, args...)
		}
		ref := func(n string) string { return reg + ":5000/smoke/" + n }
		t.waitFor(30, time.Second, reg, "artifact push failed", func() bool { _, err := or("push", "--plain-http", ref("a:v1"), "a.txt"); return err == nil })
		if out, err := or("push", "--plain-http", ref("b:v1"), "b.txt"); err != nil {
			t.Fatalf("push b: %s", out)
		}
		dig := func(n string) string {
			out, err := or("resolve", "--plain-http", ref(n+":v1"))
			if err != nil {
				t.Fatalf("resolve %s: %s", n, out)
			}
			return ref(n) + "@" + lastLine(out)
		}
		a, b := dig("a"), dig("b")
		for _, c := range [][]string{{"cert", "generate-test", "--default", "quench.smoke"}, {"sign", "--insecure-registry", "--key", "quench.smoke", a}} {
			if out, err := nt(c...); err != nil {
				t.Fatalf("notation %v: %s", c, out)
			}
		}
		os.WriteFile(filepath.Join(home, "policy.json"), []byte(`{"version":"1.0","trustPolicies":[{"name":"smoke","registryScopes":["*"],
 "signatureVerification":{"level":"strict"},"trustStores":["ca:quench.smoke"],"trustedIdentities":["*"]}]}`), 0o644)
		if out, err := nt("policy", "import", "--force", "/home/nonroot/policy.json"); err != nil {
			t.Fatalf("policy import: %s", out)
		}
		out, _ := nt("verify", "--insecure-registry", a)
		t.Contains(out, "Successfully verified", "signed artifact verifies")
		if _, err := nt("verify", "--insecure-registry", b); err == nil {
			t.Fatalf("unsigned artifact verified")
		}
		t.Logf("smoke test passed (uid %s, sign + verify, unsigned rejected)", u)
	})
	register("openscap", func(t *T) {
		u := t.Docker("inspect", t.Image, "--format", "{{.Config.User}}")
		m := regexp.MustCompile(`OpenSCAP command line tool \(oscap\) ([0-9.]+)`).FindStringSubmatch(t.Run("--version"))
		if m == nil || (t.Want != "" && m[1] != t.Want) {
			t.Fatalf("oscap version %v, expected %s", m, t.Want)
		}
		ds := t.RunEntry("/bin/bash", "-c", `f=(/usr/share/xml/scap/ssg/content/ssg-*-ds.xml); [ -e "${f[0]}" ] && echo "${f[0]}"`)
		if ds == "" {
			t.Fatalf("no scap-security-guide datastream in the image")
		}
		t.Run("ds", "sds-validate", ds)
		w := t.TempDir()
		os.WriteFile(filepath.Join(w, "def.xml"), []byte(`<?xml version="1.0" encoding="UTF-8"?>
<oval_definitions xmlns="http://oval.mitre.org/XMLSchema/oval-definitions-5"
  xmlns:oval="http://oval.mitre.org/XMLSchema/oval-common-5"
  xmlns:unix="http://oval.mitre.org/XMLSchema/oval-definitions-5#unix">
  <generator><oval:schema_version>5.11.2</oval:schema_version><oval:timestamp>2026-10-07T00:00:00</oval:timestamp></generator>
  <definitions>
    <definition class="compliance" id="oval:io.quench:def:1" version="1">
      <metadata><title>os-release present</title><description>smoke</description></metadata>
      <criteria><criterion test_ref="oval:io.quench:tst:1" comment="os-release exists"/></criteria>
    </definition>
  </definitions>
  <tests>
    <unix:file_test id="oval:io.quench:tst:1" version="1" check="all" check_existence="all_exist" comment="file exists">
      <unix:object object_ref="oval:io.quench:obj:1"/>
    </unix:file_test>
  </tests>
  <objects>
    <unix:file_object id="oval:io.quench:obj:1" version="1">
      <unix:filepath>/etc/os-release</unix:filepath>
    </unix:file_object>
  </objects>
</oval_definitions>
`), 0o644)
		out, _ := t.RunFlags([]string{"-v", w + ":/work:ro"}, "oval", "eval", "/work/def.xml")
		t.Contains(out, "oval:io.quench:def:1: true", "OVAL definition true")
		t.Logf("smoke test passed (oscap %s, SSG datastream valid, OVAL eval; user %q)", m[1], u)
	})
}
