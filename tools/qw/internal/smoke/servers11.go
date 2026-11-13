package smoke

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var noRedirect = &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

// s3Do sends one SigV4-signed S3 request (region us-east-1, path-style, no query signing
// beyond the raw query string, which must already be in canonical order).
func s3Do(method, rawURL, body, ak, sk string) (int, string) {
	req, err := http.NewRequest(method, rawURL, strings.NewReader(body))
	if err != nil {
		return 0, err.Error()
	}
	now := time.Now().UTC()
	amz, day := now.Format("20060102T150405Z"), now.Format("20060102")
	sum := sha256.Sum256([]byte(body))
	ph := hex.EncodeToString(sum[:])
	req.Header.Set("x-amz-date", amz)
	req.Header.Set("x-amz-content-sha256", ph)
	signed := "host;x-amz-content-sha256;x-amz-date"
	canon := strings.Join([]string{method, req.URL.EscapedPath(), req.URL.RawQuery,
		"host:" + req.URL.Host + "\nx-amz-content-sha256:" + ph + "\nx-amz-date:" + amz + "\n", signed, ph}, "\n")
	scope := day + "/us-east-1/s3/aws4_request"
	ch := sha256.Sum256([]byte(canon))
	sts := "AWS4-HMAC-SHA256\n" + amz + "\n" + scope + "\n" + hex.EncodeToString(ch[:])
	mac := func(k []byte, s string) []byte { h := hmac.New(sha256.New, k); h.Write([]byte(s)); return h.Sum(nil) }
	k := mac(mac(mac(mac([]byte("AWS4"+sk), day), "us-east-1"), "s3"), "aws4_request")
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+ak+"/"+scope+", SignedHeaders="+signed+", Signature="+hex.EncodeToString(mac(k, sts)))
	r, err := httpc.Do(req)
	if err != nil {
		return 0, err.Error()
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return r.StatusCode, string(b)
}

// waitCode polls url until it answers one of codes; returns the code.
func (t *T) waitCode(url string, tries int, gap time.Duration, name string, codes ...int) int {
	var c int
	for i := 0; i < tries; i++ {
		c, _ = Get(url)
		for _, w := range codes {
			if c == w {
				return c
			}
		}
		time.Sleep(gap)
	}
	t.Fatalf("%s never answered %v (last %d)\n%s", url, codes, c, t.Logs(name))
	return 0
}

func init() {
	register("node-exporter", func(t *T) {
		t.Run("--version")
		b := "http://127.0.0.1:9100"
		name, _ := t.httpUp([]string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-p", "127.0.0.1:9100:9100"}, b+"/metrics", 30)
		t.WaitHTTP(b+"/", 5, name)
		_, m := Get(b + "/metrics")
		t.Contains(m, "node_exporter_build_info", "/metrics")
		t.Logf("smoke test passed (nonroot uid: %s, read-only rootfs)", t.User("1001"))
	})
	register("storm", func(t *T) {
		v, err := t.TryRun("version")
		if err != nil {
			t.Fatalf("storm version failed:\n%s", v)
		}
		if t.Want != "" {
			t.Match(v, `(?m)^Storm `+regexp.QuoteMeta(t.Want), "version")
		}
		net_ := t.Name("net")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		c := []string{"-c", `storm.zookeeper.servers=["zk"]`, "-c", `nimbus.seeds=["nimbus"]`}
		zk, nim, ui := t.Name("zk"), t.Name("nimbus"), t.Name("ui")
		t.Start(zk, []string{"--network", net_, "--network-alias", "zk"}, "dev-zookeeper")
		t.Start(nim, []string{"--network", net_, "--network-alias", "nimbus"}, append([]string{"nimbus"}, c...)...)
		t.Start(ui, []string{"--network", net_, "-p", "8080"}, append(append([]string{"ui"}, c...), "-c", "ui.port=8080")...)
		base := "http://localhost:" + t.hostPort(ui, "8080/tcp") + "/api/v1"
		var sum string
		for i := 0; ; i++ {
			_, sum = Get(base + "/cluster/summary")
			_, n := Get(base + "/nimbus/summary")
			if strings.Contains(sum, `"stormVersion"`) && strings.Contains(n, `"Leader"`) {
				break
			}
			if i == 90 {
				t.Fatalf("cluster did not come up\n%s\n%s\n== zk\n%s\n== nimbus\n%s\n== ui\n%s", sum, n, t.Logs(zk), t.Logs(nim), t.Logs(ui))
			}
			time.Sleep(2 * time.Second)
		}
		if t.Want != "" {
			t.Contains(sum, `"stormVersion":"`+t.Want+`"`, "UI version")
		}
		t.Logf("smoke test passed (UI and Nimbus leader up; user %s)", t.User("1001"))
	})
	register("versitygw", func(t *T) {
		const ak, sk, b = "quenchtest", "quenchtestsecret0123456789", "http://127.0.0.1:7070"
		name := t.Name("")
		t.Start(name, []string{"-p", "127.0.0.1:7070:7070", "-e", "ROOT_ACCESS_KEY=" + ak, "-e", "ROOT_SECRET_KEY=" + sk})
		t.WaitHTTP(b+"/health", 30, name)
		for _, s := range [][2]string{{b + "/smoke", ""}, {b + "/smoke/hello.txt", "hello quench"}} {
			if c, r := s3Do("PUT", s[0], s[1], ak, sk); c != 200 {
				t.Fatalf("PUT %s: %d %s", s[0], c, r)
			}
		}
		if _, got := s3Do("GET", b+"/smoke/hello.txt", "", ak, sk); got != "hello quench" {
			t.Fatalf("object round-trip failed: '%s'", got)
		}
		_, list := s3Do("GET", b+"/smoke?list-type=2", "", ak, sk)
		t.Contains(list, "<Key>hello.txt</Key>", "ListObjectsV2")
		if c, _ := Get(b + "/smoke/hello.txt"); c != 403 {
			t.Fatalf("anonymous GET returned %d, expected 403", c)
		}
		m := regexp.MustCompile(`(?m)^Version\s*:?\s*(\S+)`).FindStringSubmatch(t.RunAny("--version"))
		if m == nil || m[1] == "git" || m[1] == "v0.0.0" {
			t.Fatalf("version not stamped: %v", m)
		}
		t.Logf("smoke test passed (version %s, user %s)", m[1], t.User("1001"))
	})
	register("vouch-proxy", func(t *T) {
		u := t.User("1001")
		if v := t.Run("-version"); t.Want != "" && v != "v"+t.Want {
			t.Fatalf("vouch-proxy %s, expected v%s", v, t.Want)
		}
		env := []string{"VOUCH_DOMAINS=example.com", "VOUCH_JWT_SECRET=smoke-jwt-secret-0123456789abcdef", "OAUTH_PROVIDER=oidc",
			"OAUTH_CLIENT_ID=smoke-client", "OAUTH_CLIENT_SECRET=smoke-secret", "OAUTH_AUTH_URL=https://idp.example.com/authorize",
			"OAUTH_TOKEN_URL=https://idp.example.com/token", "OAUTH_USER_INFO_URL=https://idp.example.com/userinfo",
			"OAUTH_SCOPES=openid,email", "OAUTH_CALLBACK_URL=https://vouch.example.com/auth"}
		flags := []string{"--read-only", "-p", "127.0.0.1:19090:9090"}
		for _, e := range env {
			flags = append(flags, "-e", e)
		}
		t.httpUp(flags, "http://127.0.0.1:19090/healthcheck", 30)
		req := func(host, path string) *http.Response {
			r, _ := http.NewRequest("GET", "http://127.0.0.1:19090"+path, nil)
			r.Host = host
			resp, err := noRedirect.Do(r)
			if err != nil {
				t.Fatalf("%s: %v", path, err)
			}
			resp.Body.Close()
			return resp
		}
		if c := req("app.example.com", "/validate").StatusCode; c != 401 {
			t.Fatalf("/validate without a cookie got HTTP %d", c)
		}
		loc := req("vouch.example.com", "/login?url=https://app.example.com/").Header.Get("Location")
		t.Match(loc, `^https://idp\.example\.com/authorize\?.*client_id=smoke-client`, "/login redirect")
		t.Logf("smoke test passed: healthcheck, /validate 401, /login redirects to the IdP, user %s", u)
	})
	register("xyops", func(t *T) {
		name := t.Name("")
		t.Start(name, []string{"--tmpfs", "/tmp:rw,mode=1777", "-e", "XYOPS_secret_key=smoke-test-secret-key-not-for-production", "-p", "127.0.0.1:5522:5522"})
		t.waitCode("http://127.0.0.1:5522/", 60, 2*time.Second, name, 200)
		_, body := Get("http://127.0.0.1:5522/")
		t.ContainsFold(body, "xyops", "served page")
		if c, _ := Get("http://127.0.0.1:5522/api/app/ping"); c != 200 {
			t.Fatalf("xyOps API not responding (%d)\n%s", c, t.Logs(name))
		}
		t.Logf("smoke test passed (nonroot user: %s)", t.User("1001"))
	})
	register("cilium-operator", func(t *T) {
		u := t.User("1001")
		if v, _ := t.RunFlags([]string{"--read-only", "--tmpfs", "/tmp"}, "--version"); t.Want != "" {
			t.Contains(v, t.Want, "version")
		}
		kc := t.DeadKubeconfig()
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp", "--tmpfs", "/var/run/cilium", "-v", kc + ":/etc/kubeconfig:ro"}, "--enable-gops=false", "--k8s-kubeconfig-path=/etc/kubeconfig")
		time.Sleep(20 * time.Second)
		out := t.Logs(name)
		if s := t.Docker("inspect", name, "--format", "{{.State.Running}}"); s != "true" {
			t.Fatalf("operator exited:\n%s", out)
		}
		t.Contains(out, `msg="Cilium Operator"`, "version banner")
		t.Contains(out, "127.0.0.1:1", "dialed the configured API server")
		if m := regexp.MustCompile(`panic:|SIGSEGV|read-only file system`).FindString(out); m != "" {
			t.Fatalf("operator failed (%s):\n%s", m, out)
		}
		t.Logf("smoke test passed (uid %s, up and retrying the API server)", u)
	})
	register("excalidraw", func(t *T) {
		_, body := t.httpUp([]string{"--tmpfs", "/tmp:rw,mode=1777", "-p", "127.0.0.1:8080:8080"}, "http://127.0.0.1:8080/", 30)
		t.ContainsFold(body, "excalidraw", "served page")
		if a := regexp.MustCompile(`/assets/[^"]+\.js`).FindString(body); a != "" {
			r, err := httpc.Get("http://127.0.0.1:8080" + a)
			if err != nil {
				t.Fatalf("%s: %v", a, err)
			}
			r.Body.Close()
			if ct := r.Header.Get("Content-Type"); !regexp.MustCompile(`javascript|ecmascript`).MatchString(ct) {
				t.Fatalf("JS asset served with wrong content-type: '%s'", ct)
			}
		}
		t.Logf("smoke test passed (nonroot user: %s)", t.User("1001"))
	})
	register("ghost", func(t *T) {
		name := t.Name("")
		t.Start(name, []string{"--tmpfs", "/tmp:rw,mode=1777", "-e", "NODE_ENV=development", "-e", "url=http://127.0.0.1:2368", "-p", "127.0.0.1:2368:2368"})
		t.waitCode("http://127.0.0.1:2368/", 60, 2*time.Second, name, 200)
		_, body := Get("http://127.0.0.1:2368/")
		r, err := noRedirect.Get("http://127.0.0.1:2368/ghost/")
		hdr := ""
		if err == nil {
			r.Body.Close()
			for k, v := range r.Header {
				hdr += k + ": " + strings.Join(v, ",") + "\n"
			}
		}
		t.ContainsFold(hdr+body, "ghost", "served page looks like Ghost")
		t.Logf("smoke test passed (nonroot user: %s)", t.User("1001"))
	})
	register("jenkins", func(t *T) {
		u := t.User("1001")
		t.RunEntry("/usr/lib/jvm/java-21-openjdk/bin/java", "-jar", "/usr/share/jenkins/jenkins.war", "--version")
		name := t.Name("")
		t.Start(name, []string{"-p", "127.0.0.1:0:8080"})
		c := t.waitCode("http://127.0.0.1:"+t.hostPort(name, "8080/tcp")+"/login", 75, 2*time.Second, name, 200, 403)
		t.Logf("smoke test passed (nonroot user: %s, login HTTP %d)", u, c)
	})
	register("metabase", func(t *T) {
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/data:rw,uid=1001,gid=1001", "--tmpfs", "/tmp:rw,exec,mode=1777", "-p", "127.0.0.1:13300:3000"})
		for i := 0; ; i++ {
			if _, h := Get("http://127.0.0.1:13300/api/health"); strings.Contains(h, `"status":"ok"`) {
				break
			}
			if s := t.Docker("inspect", "-f", "{{.State.Status}}", name); s != "running" {
				t.Fatalf("metabase died during startup:\n%s", t.Logs(name))
			}
			if i == 120 {
				t.Fatalf("health never reported ok\n%s", t.Logs(name))
			}
			time.Sleep(3 * time.Second)
		}
		_, props := Get("http://127.0.0.1:13300/api/session/properties")
		var p struct {
			Version struct{ Tag string } `json:"version"`
		}
		if json.Unmarshal([]byte(props), &p) != nil || p.Version.Tag == "" {
			t.Fatalf("no version tag in session properties")
		}
		if t.Want != "" && p.Version.Tag != "v"+t.Want {
			t.Fatalf("version %s, expected v%s", p.Version.Tag, t.Want)
		}
		if m := regexp.MustCompile(`Read-only file system|AccessDenied|UnsatisfiedLinkError`).FindString(t.Logs(name)); m != "" {
			t.Fatalf("write or native-load errors in logs: %s", m)
		}
		t.Logf("smoke test passed (nonroot %s, read-only rootfs, API up, %s)", t.User("1001"), p.Version.Tag)
	})
}
