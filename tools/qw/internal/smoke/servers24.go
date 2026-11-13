package smoke

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func init() {
	register("plone", func(t *T) {
		const b = "http://127.0.0.1:18080"
		u := t.User("1001")
		v := t.RunEntry("/opt/plone/venv/bin/python", "-c", "import importlib.metadata as m; print(m.version('Plone'))")
		if t.Want != "" && v != t.Want {
			t.Fatalf("version mismatch: %s", v)
		}
		vol, name := t.Name("data"), t.Name("")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		boot := func() {
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp", "-v", vol + ":/data", "-e", "PLONE_ADMIN_PASSWORD=smoke-pw", "-p", "127.0.0.1:18080:8080"})
			t.waitFor(120, 2*time.Second, name, "plone never answered", func() bool { c, _ := Get(b + "/"); return c/100 == 2 })
		}
		api := func() {
			j := map[string]string{"Accept": "application/json"}
			_, site := DoH("GET", b+"/Plone/++api++", "", j)
			t.Match(site, `"@id": *"`+regexp.QuoteMeta(b)+`/Plone"`, "site JSON")
			t.Match(site, `"@type": *"Plone Site"`, "Plone Site")
			_, adm := DoH("GET", "http://admin:smoke-pw@127.0.0.1:18080/Plone/++api++/@users/admin", "", j)
			t.Match(adm, `"username": *"admin"`, "admin authenticated")
		}
		boot()
		api()
		logs := t.Logs(name)
		t.Contains(logs, "created Plone site /Plone", "site created on first boot")
		if regexp.MustCompile(`Traceback|CRITICAL`).MatchString(logs) {
			t.Fatalf("plone logged errors\n%s", logs)
		}
		t.Docker("rm", "-f", name)
		boot()
		api()
		if strings.Contains(t.Logs(name), "created Plone site") {
			t.Fatalf("site recreated on restart: Data.fs not reused")
		}
		t.Logf("smoke test passed (plone %s, uid %s, site created, API served, reused after restart)", v, u)
	})
	register("pulsar", func(t *T) {
		const admin = "http://127.0.0.1:8080/admin/v2"
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/pulsar:rw,exec,mode=1777,size=1g", "-p", "127.0.0.1:8080:8080", "-p", "127.0.0.1:6650:6650"})
		t.waitFor(90, 2*time.Second, name, "pulsar did not become ready", func() bool { c, _ := Get(admin + "/brokers/health"); return c == 200 })
		_, cl := Get(admin + "/clusters")
		t.Contains(cl, "standalone", "standalone cluster")
		if out, err := docker("exec", name, "bin/pulsar-client", "produce", "-m", "quench", "-n", "1", "persistent://public/default/gate"); err != nil {
			t.Fatalf("produce failed: %s", out)
		}
		out, _ := docker("exec", name, "bin/pulsar-client", "consume", "-s", "sub", "-p", "Earliest", "-n", "1", "persistent://public/default/gate")
		t.Contains(out, "quench", "message round trip")
		t.Logf("smoke test passed (nonroot user: %s)", t.User("1001"))
	})
	register("questdb", func(t *T) {
		name := t.Name("")
		t.Start(name, []string{"-p", "19000:9000"})
		q := func(sql string) (int, string) {
			return Get("http://127.0.0.1:19000/exec?" + url.Values{"query": {sql}}.Encode())
		}
		t.waitFor(60, 2*time.Second, name, "questdb did not become ready", func() bool { c, _ := q("SELECT 1"); return c == 200 })
		q("CREATE TABLE smoke(ts TIMESTAMP, v DOUBLE) TIMESTAMP(ts) PARTITION BY DAY")
		q("INSERT INTO smoke VALUES(now(), 0.64)")
		t.waitFor(20, time.Second, name, "expected 1 row", func() bool { _, r := q("SELECT count() FROM smoke"); return strings.Contains(r, "[[1]]") })
		if _, bld := q("SELECT build()"); t.Want != "" {
			t.Contains(bld, t.Want, "build() version")
		}
		t.Logf("smoke test passed (nonroot user: %s)", t.User("1001"))
	})
	register("rekor-tiles", func(t *T) {
		u := t.User("1001")
		t.Match(t.RunEntry("/usr/bin/rekor-server", "version"), `GitVersion: *v`+regexp.QuoteMeta(t.Want), "version")
		w := t.TempDir()
		_, sk, _ := ed25519.GenerateKey(rand.Reader)
		der, _ := x509.MarshalPKCS8PrivateKey(sk)
		os.WriteFile(filepath.Join(w, "signer.pem"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o644)
		name := t.Name("")
		t.Start(name, []string{"-p", "127.0.0.1:13000:3000", "-v", w + "/signer.pem:/pki/signer.pem:ro"},
			"--hostname=rekor.smoke", "--signer-filepath=/pki/signer.pem", "--checkpoint-interval=1s")
		h := t.WaitHTTP("http://127.0.0.1:13000/healthz", 30, name)
		t.Contains(h, `"status":"SERVING"`, "healthz")
		art := []byte("quench smoke artifact\n")
		d := sha256.Sum256(art)
		ek, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		sig, _ := ecdsa.SignASN1(rand.Reader, ek, d[:])
		pub, _ := x509.MarshalPKIXPublicKey(&ek.PublicKey)
		e := base64.StdEncoding.EncodeToString
		req := `{"hashedRekordRequestV002":{"digest":"` + e(d[:]) + `","signature":{"content":"` + e(sig) +
			`","verifier":{"publicKey":{"rawBytes":"` + e(pub) + `"},"keyDetails":"PKIX_ECDSA_P256_SHA_256"}}}}`
		c, resp := Post("http://127.0.0.1:13000/api/v2/log/entries", "application/json", req)
		if c != 201 {
			t.Fatalf("entry rejected: HTTP %d: %s\n%s", c, resp, t.Logs(name))
		}
		t.Contains(resp, `"logIndex":"0"`, "log index")
		t.Contains(resp, `"inclusionProof"`, "inclusion proof")
		t.Logf("smoke test passed (healthz, signed entry with inclusion proof; nonroot user: %s)", u)
	})
	register("sozu", func(t *T) {
		u := t.User("1001")
		v := t.Run("--version")
		if t.Want != "" {
			t.Contains(v, t.Want, "version")
		}
		net_, sz, be := t.Name("net"), t.Name(""), t.Name("be")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Cleanup(func() { docker("rm", "-f", be) })
		t.Docker("run", "-d", "--name", be, "--network", net_, "--entrypoint", "/usr/bin/caddy", "ghcr.io/quenchworks/images/caddy:2.11.9", "respond", "--listen", ":8000", "quench-backend")
		t.Start(sz, []string{"--network", net_, "-p", "127.0.0.1::8080"})
		ctl := func(args ...string) (string, error) {
			return docker(append([]string{"exec", sz, "/usr/bin/sozu", "-c", "/etc/sozu/config.toml"}, args...)...)
		}
		t.waitFor(30, time.Second, sz, "sozu did not come up", func() bool { _, err := ctl("status"); return err == nil })
		ip := t.Docker("inspect", be, "--format", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}")
		for _, c := range [][]string{{"cluster", "add", "--id", "smoke", "--load-balancing-policy", "round_robin"},
			{"backend", "add", "--id", "smoke", "--backend-id", "smoke-0", "--address", ip + ":8000"},
			{"frontend", "http", "add", "--address", "0.0.0.0:8080", "--hostname", "smoke.local", "id", "smoke"}} {
			if out, err := ctl(c...); err != nil {
				t.Fatalf("sozu %v: %s", c, out)
			}
		}
		base := "http://127.0.0.1:" + t.hostPort(sz, "8080/tcp") + "/"
		get := func(host string) (int, string) {
			r, _ := hostReq("GET", base, host)
			resp, err := httpc.Do(r)
			if err != nil {
				return 0, err.Error()
			}
			defer resp.Body.Close()
			b := make([]byte, 256)
			n, _ := resp.Body.Read(b)
			return resp.StatusCode, string(b[:n])
		}
		t.waitFor(15, time.Second, sz, "proxying failed", func() bool { _, b := get("smoke.local"); return b == "quench-backend" })
		if c, _ := get("unknown.local"); c != 404 {
			t.Fatalf("unknown host should get 404, got %d", c)
		}
		t.Logf("smoke test passed (%s, nonroot user: %s)", v, u)
	})
	register("tetragon", func(t *T) {
		tv := t.RunEntryAny("/usr/bin/tetra", "version")
		t.Match(tv, `[0-9]+\.[0-9]+\.[0-9]+`, "tetra version")
		if t.Want != "" {
			t.Contains(tv, t.Want, "tetra version")
		}
		name := t.Name("")
		mark := t.Name("mark")
		t.Start(name, []string{"--privileged", "--pid=host", "--user", "0", "-v", "/sys/fs/bpf:/sys/fs/bpf", "--tmpfs", "/var/run/tetragon", "--tmpfs", "/tmp"},
			"--export-filename", "/tmp/events.log", "--log-level", "info")
		t.waitFor(90, 2*time.Second, name, "tetra status never reported running", func() bool {
			if s := t.Docker("inspect", "-f", "{{.State.Status}}", name); s != "running" {
				t.Fatalf("agent died during startup\n%s", t.Logs(name))
			}
			st, _ := docker("exec", name, "/usr/bin/tetra", "status")
			return strings.Contains(strings.ToLower(st), "running")
		})
		sv, _ := docker("exec", name, "/usr/bin/tetra", "version", "--server")
		t.Match(sv, `(?i)server version`, "agent version over gRPC")
		if t.Want != "" {
			t.Contains(sv, t.Want, "agent version")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		ev := exec.CommandContext(ctx, "docker", "exec", name, "/usr/bin/tetra", "getevents", "-o", "compact")
		var buf strings.Builder
		ev.Stdout, ev.Stderr = &buf, &buf
		ev.Start()
		time.Sleep(3 * time.Second)
		docker("run", "--rm", "--entrypoint", "/usr/bin/tetra", t.Image, "help", mark)
		ev.Wait()
		found := false
		for _, l := range strings.Split(buf.String(), "\n") {
			found = found || (strings.Contains(l, mark) && strings.Contains(l, "process"))
		}
		if !found {
			t.Fatalf("no process event for the marker process:\n%.2000s", buf.String())
		}
		t.Logf("smoke test passed (BPF sensors loaded, exec events streamed)")
	})
	register("unleash", func(t *T) {
		const b = "http://127.0.0.1:14242"
		u := t.User("1001")
		net_, pg, name := t.Name("net"), t.Name("pg"), t.Name("")
		pw, adminPW := t.Name("pg"), t.Name("Quench")+"-Pw1"
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Cleanup(func() { docker("rm", "-f", pg) })
		t.Docker("run", "-d", "--name", pg, "--network", net_, "-e", "POSTGRES_PASSWORD="+pw, "ghcr.io/quenchworks/images/postgresql:18.6")
		t.waitFor(60, 2*time.Second, pg, "postgres did not become ready", func() bool {
			_, err := docker("exec", "-e", "PGPASSWORD="+pw, pg, "psql", "-h", "127.0.0.1", "-U", "postgres", "-tAc", "select 1")
			return err == nil
		})
		t.Start(name, []string{"--network", net_, "--read-only", "--tmpfs", "/tmp", "-p", "127.0.0.1:14242:4242",
			"-e", "DATABASE_URL=postgres://postgres:" + pw + "@" + pg + ":5432/postgres", "-e", "DATABASE_SSL=false",
			"-e", "UNLEASH_DEFAULT_ADMIN_USERNAME=admin", "-e", "UNLEASH_DEFAULT_ADMIN_PASSWORD=" + adminPW,
			"-e", "CHECK_VERSION=false", "-e", "SEND_TELEMETRY=false"})
		t.waitFor(120, time.Second, name, "unleash never reported healthy", func() bool { _, h := Get(b + "/health"); return strings.Contains(h, `"health":"GOOD"`) })
		jar, _ := cookiejar.New(nil)
		c := &http.Client{Timeout: 10 * time.Second, Jar: jar}
		do := func(method, path, body string) (int, string) {
			req, _ := http.NewRequest(method, b+path, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r, err := c.Do(req)
			if err != nil {
				return 0, err.Error()
			}
			defer r.Body.Close()
			out := new(strings.Builder)
			buf := make([]byte, 64<<10)
			for {
				n, err := r.Body.Read(buf)
				out.Write(buf[:n])
				if err != nil {
					break
				}
			}
			return r.StatusCode, out.String()
		}
		if code, r := do("POST", "/auth/simple/login", `{"username":"admin","password":"`+adminPW+`"}`); code/100 != 2 {
			t.Fatalf("admin login: %d %s", code, r)
		}
		if t.Want != "" {
			_, cfg := do("GET", "/api/admin/ui-config", "")
			t.Contains(cfg, `"version":"`+t.Want+`"`, "version")
		}
		if code, r := do("POST", "/api/admin/projects/default/features", `{"name":"quench-smoke","type":"release"}`); code/100 != 2 {
			t.Fatalf("flag create: %d %s", code, r)
		}
		_, f := do("GET", "/api/admin/projects/default/features/quench-smoke", "")
		t.Contains(f, `"name":"quench-smoke"`, "flag read back")
		if code, _ := Post(b+"/auth/simple/login", "application/json", `{"username":"admin","password":"wrong"}`); code < 400 {
			t.Fatalf("a wrong password got HTTP %d", code)
		}
		if m := regexp.MustCompile(`"level":"(error|fatal)"|\[ERROR\]|FATAL`).FindString(t.Logs(name)); m != "" {
			t.Fatalf("unleash logged errors\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed (uid %s, migrated, admin login, flag created and read)", u)
	})
}
