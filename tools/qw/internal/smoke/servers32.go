package smoke

import (
	"archive/zip"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

func init() {
	register("linkerd-extension-init", func(t *T) {
		d := t.TempDir()
		caAndServer(d, "host.docker.internal")
		cert, err := tls.LoadX509KeyPair(filepath.Join(d, "serverCert.pem"), filepath.Join(d, "serverKey.pem"))
		if err != nil {
			t.Fatalf("server cert: %v", err)
		}
		var mu sync.Mutex
		var patch []byte
		ns := `{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"linkerd-viz"}}`
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch r.Method + " " + r.URL.Path {
			case "GET /api/v1/namespaces/linkerd-viz":
				io.WriteString(w, ns)
			case "PATCH /api/v1/namespaces/linkerd-viz":
				b, _ := io.ReadAll(r.Body)
				mu.Lock()
				patch = b
				mu.Unlock()
				io.WriteString(w, ns)
			case "GET /api/v1/namespaces/linkerd/configmaps/linkerd-config":
				io.WriteString(w, `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"linkerd-config","namespace":"linkerd"},"data":{"values":"cniEnabled: true\n"}}`)
			default:
				w.WriteHeader(404)
				io.WriteString(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"NotFound","code":404}`)
			}
		})
		ln, err := tls.Listen("tcp", "0.0.0.0:0", &tls.Config{Certificates: []tls.Certificate{cert}})
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		go http.Serve(ln, h)
		t.Cleanup(func() { ln.Close() })
		ca, _ := os.ReadFile(filepath.Join(d, "caCert.pem"))
		os.WriteFile(filepath.Join(d, "kubeconfig"), []byte(`apiVersion: v1
kind: Config
clusters: [{name: fake, cluster: {server: "https://host.docker.internal:`+strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)+`", certificate-authority-data: "`+base64.StdEncoding.EncodeToString(ca)+`"}}]
users: [{name: fake, user: {token: fake-token}}]
contexts: [{name: fake, context: {cluster: fake, user: fake}}]
current-context: fake
`), 0o644)
		out, code := t.RunAnyFor(60*time.Second, append([]string{"--read-only", "-v", d + "/kubeconfig:/cfg/kubeconfig:ro", "-e", "KUBECONFIG=/cfg/kubeconfig"}, hostStubFlag...),
			"--log-format", "plain", "--log-level", "info", "--extension", "viz", "--namespace", "linkerd-viz", "--linkerd-namespace", "linkerd")
		if code != 0 {
			t.Fatalf("extension-init exited %d:\n%s", code, out)
		}
		mu.Lock()
		p := patch
		mu.Unlock()
		var ops []struct {
			Op, Path string
			Value    any
		}
		if json.Unmarshal(p, &ops) != nil {
			t.Fatalf("no JSON patch reached the API server: %q", p)
		}
		got := map[string]any{}
		for _, o := range ops {
			if o.Op == "add" {
				got[o.Path] = o.Value
			}
		}
		if got["/metadata/labels/linkerd.io~1extension"] != "viz" || got["/metadata/labels/pod-security.kubernetes.io~1enforce"] != "restricted" {
			t.Fatalf("PATCH lacks the extension and pod-security labels: %s", p)
		}
		t.Contains(out, "successfully patched namespace", "success log line")
		t.Logf("smoke test passed (namespace labelled through a verified-TLS fake API server)")
	})
	register("polaris", func(t *T) {
		d := t.TempDir()
		os.WriteFile(filepath.Join(d, "deploy.yaml"), []byte(`apiVersion: apps/v1
kind: Deployment
metadata: {name: weak, namespace: smoke}
spec:
  selector: {matchLabels: {app: weak}}
  template:
    metadata: {labels: {app: weak}}
    spec:
      containers:
        - name: app
          image: nginx:latest
          securityContext: {privileged: true}
`), 0o644)
		out, err := exec2out(t, "run", "--rm", "-v", d+":/audit:ro", t.Image, "audit", "--audit-path", "/audit", "--format", "json")
		if err != nil {
			t.Fatalf("audit: %v", err)
		}
		var a struct {
			Results []struct {
				Name      string
				PodResult struct {
					ContainerResults []struct {
						Results map[string]struct{ Success bool }
					}
				}
			}
		}
		if json.Unmarshal([]byte(out), &a) != nil {
			t.Fatalf("audit output is not JSON: %.400s", out)
		}
		var res map[string]struct{ Success bool }
		for _, r := range a.Results {
			if r.Name == "weak" && len(r.PodResult.ContainerResults) > 0 {
				res = r.PodResult.ContainerResults[0].Results
				break
			}
		}
		for _, c := range []string{"runAsPrivileged", "tagNotSpecified", "livenessProbeMissing"} {
			if r, ok := res[c]; !ok || r.Success {
				t.Fatalf("audit did not flag %s", c)
			}
		}
		name := t.Name("")
		t.Start(name, []string{"-p", "127.0.0.1:8080:8080", "-v", d + ":/audit:ro"}, "dashboard", "--port", "8080", "--audit-path", "/audit")
		t.WaitHTTP("http://127.0.0.1:8080/health", 30, name)
		_, rj := Get("http://127.0.0.1:8080/results.json")
		t.Contains(rj, `"Name":"weak"`, "dashboard results")
		_, page := Get("http://127.0.0.1:8080/")
		t.Contains(page, "<title>Fairwinds Polaris</title>", "dashboard UI")
		v := regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+`).FindString(t.RunAny("version"))
		if v == "" {
			t.Fatalf("version not stamped")
		}
		t.Logf("smoke test passed (version %s, user %s)", v, t.Docker("inspect", t.Image, "--format", "{{.Config.User}}"))
	})
	register("wildfly", func(t *T) {
		u := t.User("1001")
		d := t.TempDir()
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		f, _ := zw.Create("index.jsp")
		io.WriteString(f, `<%= "quench-" + (6*7) %>`+"\n")
		zw.Close()
		war := filepath.Join(d, "quench.war")
		os.WriteFile(war, buf.Bytes(), 0o644)
		name := t.Name("")
		var logs, started string
		boot := func(args ...string) {
			docker("rm", "-f", name)
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", war + ":/opt/wildfly/standalone/deployments/quench.war:ro", "-p", "127.0.0.1:8080:8080"}, args...)
			t.waitFor(120, time.Second, name, "WildFly did not finish booting", func() bool {
				logs = t.Logs(name)
				return regexp.MustCompile(`WFLYSRV002[56]`).MatchString(logs)
			})
			if strings.Contains(logs, "WFLYSRV0026") {
				t.Fatalf("WildFly started WITH ERRORS\n%s", logs)
			}
			started = regexp.MustCompile(`WildFly [0-9.]+Final`).FindString(logs)
			if t.Want != "" && started != "WildFly "+t.Want+".Final" {
				t.Fatalf("started %q, expected WildFly %s.Final", started, t.Want)
			}
			t.Contains(logs, "Http management interface listening on http://127.0.0.1:9990", "management bound to loopback only")
			t.waitFor(30, time.Second, name, "the deployed JSP did not answer quench-42", func() bool {
				_, b := Get("http://127.0.0.1:8080/quench/")
				return strings.TrimSpace(b) == "quench-42"
			})
		}
		boot()
		if c, _ := Get("http://127.0.0.1:8080/"); c != 200 {
			t.Fatalf("welcome content not served at / (HTTP %d)", c)
		}
		boot("-c", "standalone-microprofile.xml")
		t.Contains(logs, "WFLYMPTEL0001", "MicroProfile Telemetry subsystem")
		t.Logf("smoke test passed (%s, JSP deployed on both configs, telemetry up, nonroot user: %s)", started, u)
	})
	register("zipkin", func(t *T) {
		const b = "http://127.0.0.1:19411"
		u := t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp", "-p", "127.0.0.1:19411:9411"})
		up := regexp.MustCompile(`"status" *: *"UP"`)
		t.waitFor(120, time.Second, name, "zipkin never reported UP", func() bool { _, h := Get(b + "/health"); return up.MatchString(h) })
		_, info := Get(b + "/info")
		if t.Want != "" {
			t.Contains(info, `"version":"`+t.Want+`"`, "version")
		}
		if c, r := Post(b+"/api/v2/spans", "application/json", `[{"traceId":"5af7183fb1d4cf5f","id":"6b221d5bc9e6496c","name":"get /","timestamp":1790000000000000,"duration":2000,"localEndpoint":{"serviceName":"quench-smoke"}}]`); c != 202 {
			t.Fatalf("span POST returned %d %s", c, r)
		}
		t.waitFor(20, time.Second, name, "service not listed", func() bool { _, s := Get(b + "/api/v2/services"); return strings.Contains(s, "quench-smoke") })
		_, tr := Get(b + "/api/v2/trace/5af7183fb1d4cf5f")
		t.Contains(tr, `"id":"6b221d5bc9e6496c"`, "trace returned")
		_, ui := Get(b + "/zipkin/")
		t.ContainsFold(ui, "<html", "UI")
		if regexp.MustCompile(` ERROR |Exception`).MatchString(t.Logs(name)) {
			t.Fatalf("server logged errors\n%s", t.Logs(name))
		}
		// HTTPS: a PKCS12 keystore from the image's own keytool, verified against its cert
		w := t.TempDir()
		kt := []string{"-v", w + ":/w", "--entrypoint", "/usr/lib/jvm/java-21-openjdk/bin/keytool"}
		if out, err := t.RunFlags(kt, "-genkeypair", "-alias", "z", "-keyalg", "RSA", "-keysize", "2048", "-dname", "CN=localhost", "-ext", "SAN=dns:localhost",
			"-validity", "1", "-storetype", "PKCS12", "-keystore", "/w/ks.p12", "-storepass", "changeit"); err != nil {
			t.Fatalf("keytool genkeypair: %s", out)
		}
		if out, err := t.RunFlags(kt, "-exportcert", "-rfc", "-alias", "z", "-keystore", "/w/ks.p12", "-storepass", "changeit", "-file", "/w/c.pem"); err != nil {
			t.Fatalf("keytool exportcert: %s", out)
		}
		pemb, _ := os.ReadFile(filepath.Join(w, "c.pem"))
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pemb) {
			t.Fatalf("exported cert unreadable")
		}
		tn := t.Name("tls")
		t.Docker("create", "--name", tn, "-p", "127.0.0.1:19443:9411", "-e", "ARMERIA_SSL_ENABLED=true", "-e", "ARMERIA_SSL_KEY_STORE=file:/ks.p12",
			"-e", "ARMERIA_SSL_KEY_STORE_PASSWORD=changeit", "-e", "ARMERIA_SSL_KEY_STORE_TYPE=PKCS12", "-e", "ARMERIA_PORTS_0_PORT=9411",
			"-e", "ARMERIA_PORTS_0_PROTOCOLS=https", t.Image)
		t.Cleanup(func() { docker("rm", "-f", tn) })
		t.Docker("cp", filepath.Join(w, "ks.p12"), tn+":/ks.p12")
		t.Docker("start", tn)
		hc := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}}
		t.waitFor(60, time.Second, tn, "HTTPS health failed", func() bool { _, _, h := getH(hc, "https://localhost:19443/health"); return up.MatchString(h) })
		t.Logf("smoke test passed (span stored and queried, HTTPS health verified; uid %s)", u)
	})
}

// exec2out runs docker and returns stdout only.
func exec2out(t *T, args ...string) (string, error) {
	b, err := exec.Command("docker", args...).Output()
	return string(b), err
}
