package smoke

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func init() {
	// nebula-cert from the image makes a CA, a lighthouse and a host certificate. A lighthouse and
	// a host (both with tun.disabled, so no NET_ADMIN) start from those, and the host must complete
	// a handshake with the lighthouse over UDP. A host signed by another CA must be refused. 1.12
	// binds a control socket in /var/run/nebula, so /run is a tmpfs as it is an emptyDir in a pod.
	register("nebula", func(t *T) {
		t.User("1001")
		v := t.RunAny("-version")
		t.Logf("%s", v)
		t.WantVersion(v)

		w := t.TempDir()
		cert := func(args ...string) {
			t.Docker(append([]string{"run", "--rm", "-v", w + ":/c", "-w", "/c", "--entrypoint", "/usr/bin/nebula-cert", t.Image}, args...)...)
		}
		cert("ca", "-name", "smoke-ca")
		cert("sign", "-name", "lh", "-ip", "192.168.100.1/24")
		cert("sign", "-name", "host", "-ip", "192.168.100.2/24")
		cert("ca", "-name", "rogue-ca", "-out-crt", "rogue-ca.crt", "-out-key", "rogue-ca.key")
		cert("sign", "-name", "rogue", "-ip", "192.168.100.3/24", "-ca-crt", "rogue-ca.crt", "-ca-key", "rogue-ca.key")

		net, lh, host, rogue := t.Name("net"), t.Name("lh"), t.Name("host"), t.Name("rogue")
		conf := func(name, ca string, isLH bool) {
			hosts := `["192.168.100.1"]`
			if isLH {
				hosts = "[]"
			}
			y := fmt.Sprintf(`pki: { ca: /c/%s, cert: /c/%s.crt, key: /c/%s.key }
listen: { host: 0.0.0.0, port: 4242 }
tun: { disabled: true }
lighthouse:
  am_lighthouse: %t
  hosts: %s
static_host_map: { "192.168.100.1": ["%s:4242"] }
firewall:
  outbound: [{ port: any, proto: any, host: any }]
  inbound: [{ port: any, proto: any, host: any }]
`, ca, name, name, isLH, hosts, lh)
			if err := os.WriteFile(filepath.Join(w, name+".yml"), []byte(y), 0o644); err != nil {
				t.Fatalf("write %s.yml: %v", name, err)
			}
		}
		conf("lh", "ca.crt", true)
		conf("host", "ca.crt", false)
		conf("rogue", "rogue-ca.crt", false)
		filepath.Walk(w, func(p string, _ os.FileInfo, _ error) error { os.Chmod(p, 0o755); return nil })

		t.Docker("network", "create", net)
		t.Cleanup(func() { docker("network", "rm", net) })
		flags := func() []string {
			return []string{"--network", net, "--read-only", "--tmpfs", "/run:uid=1001,gid=1001", "-v", w + ":/c:ro"}
		}
		t.Start(lh, flags(), "-config", "/c/lh.yml")
		time.Sleep(2 * time.Second)
		t.Start(host, flags(), "-config", "/c/host.yml")
		t.Start(rogue, flags(), "-config", "/c/rogue.yml")
		t.WaitLog(host, "Handshake message received", 30)
		time.Sleep(3 * time.Second)

		if strings.Contains(t.Logs(rogue), "Handshake message received") {
			t.Fatalf("a host from another CA was let in")
		}
		lhLog := t.Logs(lh)
		t.Match(lhLog, `(?i)could not find ca|certificate validation failed|not signed by|could not verify|untrusted`, "lighthouse rejecting the rogue certificate")
		if regexp.MustCompile(`panic|read-only file system`).MatchString(lhLog) {
			t.Fatalf("lighthouse crashed or wrote to the root:\n%s", lhLog)
		}
		t.Logf("smoke test passed: CA and certs from nebula-cert, lighthouse handshake, foreign CA refused, user 1001")
	})
}

func init() {
	// Real Git service work on a read-only root with /data on a tmpfs: Gogs starts from a locked
	// config (SQLite, built-in SSH), an admin is created from the CLI, a repository is
	// auto-initialised through the API (git in the image), and a clone plus push over HTTP runs
	// the server-side hooks (bash in the image) and shows up through the API.
	register("gogs", func(t *T) {
		u := t.User("1001")
		v, err := t.RunFlags([]string{"--read-only", "--tmpfs", "/tmp"}, "--version")
		if err != nil {
			t.Fatalf("--version: %v\n%s", err, v)
		}
		t.WantVersion(v)
		w := t.TempDir()
		ini := fmt.Sprintf(`RUN_USER = git
RUN_MODE = prod
[server]
HTTP_PORT = 3000
EXTERNAL_URL = http://127.0.0.1:3000/
START_SSH_SERVER = true
SSH_PORT = 2222
SSH_LISTEN_PORT = 2222
[database]
TYPE = sqlite3
PATH = /data/gogs.db
[repository]
ROOT = /data/repositories
[security]
INSTALL_LOCK = true
SECRET_KEY = smoke-%d
[log]
MODE = console
LEVEL = Info
`, time.Now().UnixNano())
		if err := os.WriteFile(filepath.Join(w, "app.ini"), []byte(ini), 0o644); err != nil {
			t.Fatalf("write app.ini: %v", err)
		}
		name := t.Name("")
		// exec on /data: the hooks Gogs writes into each repository run from there
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp", "--tmpfs", "/data:exec,uid=1001,gid=1001",
			"-v", w + "/app.ini:/etc/gogs/app.ini:ro", "-p", "127.0.0.1::3000"}, "web", "--config", "/etc/gogs/app.ini")
		port := t.hostPort(name, "3000")
		base := "http://127.0.0.1:" + port
		t.waitFor(60, time.Second, name, "gogs never became healthy", func() bool {
			_, b := Get(base + "/healthcheck")
			return strings.Contains(b, "Database connection: OK")
		})
		t.Contains(t.Logs(name), "SSH server started", "built-in SSH server")

		pw := fmt.Sprintf("Quench-smoke-%d", time.Now().UnixNano()%100000)
		t.Docker("exec", name, "/usr/bin/gogs", "admin", "create-user", "--name", "quench", "--password", pw,
			"--email", "smoke@example.com", "--admin", "--config", "/etc/gogs/app.ini")
		basic := "Basic " + base64.StdEncoding.EncodeToString([]byte("quench:"+pw))
		code, body := DoH("POST", base+"/api/v1/users/quench/tokens", `{"name":"smoke"}`,
			map[string]string{"Authorization": basic, "Content-Type": "application/json"})
		var tk struct{ Sha1 string }
		if json.Unmarshal([]byte(body), &tk); code != 201 || tk.Sha1 == "" {
			t.Fatalf("no API token (%d): %s", code, body)
		}
		auth := map[string]string{"Authorization": "token " + tk.Sha1, "Content-Type": "application/json"}
		if code, body := DoH("POST", base+"/api/v1/user/repos", `{"name":"smoke","auto_init":true,"readme":"Default"}`, auth); code != 201 {
			t.Fatalf("create repo: %d %s", code, body)
		}
		_, readme := DoH("GET", base+"/api/v1/repos/quench/smoke/raw/master/README.md", "", auth)
		t.Contains(readme, "# smoke", "auto-init README")

		clone := filepath.Join(w, "clone")
		git := func(args ...string) {
			if out, err := runHost("git", append([]string{"-C", clone, "-c", "user.email=smoke@example.com", "-c", "user.name=smoke"}, args...)...); err != nil {
				t.Fatalf("git %v: %v\n%s\n--- logs:\n%s", args, err, out, t.Logs(name))
			}
		}
		if out, err := runHost("git", "clone", "-q", "http://quench:"+pw+"@127.0.0.1:"+port+"/quench/smoke.git", clone); err != nil {
			t.Fatalf("clone: %v\n%s", err, out)
		}
		os.WriteFile(filepath.Join(clone, "pushed.txt"), []byte("quench\n"), 0o644)
		git("add", "pushed.txt")
		git("commit", "-qm", "push")
		git("push", "-q", "origin", "master")
		if _, b := DoH("GET", base+"/api/v1/repos/quench/smoke/raw/master/pushed.txt", "", auth); strings.TrimSpace(b) != "quench" {
			t.Fatalf("pushed file not served: %q\n--- logs:\n%s", b, t.Logs(name))
		}
		if m := regexp.MustCompile(`(?m)^.*(\[ERROR\]|\[FATAL\]|panic:).*$`).FindString(t.Logs(name)); m != "" {
			t.Fatalf("gogs logged errors: %s", m)
		}
		t.Logf("smoke test passed (gogs %s, uid %s, repo created, clone and push through the hooks)", t.Want, u)
	})
}

func init() {
	// Creates an HTTPServer and a Pipeline through the admin API, then sends a request through
	// the new traffic port: the Pipeline's Proxy filter forwards it to the admin API itself, so a
	// reply proves the data path. Objects live in the embedded etcd and survive a restart.
	register("easegress", func(t *T) {
		v := t.RunAny("--version")
		t.Logf("reported: %s", v)
		t.WantVersionV(v)
		u := t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"-p", "127.0.0.1::2381", "-p", "127.0.0.1::10080"})
		admin := "http://127.0.0.1:" + t.hostPort(name, "2381") + "/apis/v2"
		t.WaitHTTP(admin+"/healthz", 60, name)
		yaml := map[string]string{"Content-Type": "text/x-yaml"}
		for _, obj := range []string{`name: qw-pipeline
kind: Pipeline
flow:
  - filter: proxy
filters:
  - name: proxy
    kind: Proxy
    pools:
      - servers:
          - url: http://127.0.0.1:2381
`, `name: qw-server
kind: HTTPServer
port: 10080
rules:
  - paths:
      - pathPrefix: /apis
        backend: qw-pipeline
`} {
			if code, body := DoH("POST", admin+"/objects", obj, yaml); code/100 != 2 {
				t.Fatalf("creating an object failed: %d %s\n--- logs:\n%s", code, body, t.Logs(name))
			}
		}
		got := t.WaitHTTP("http://127.0.0.1:"+t.hostPort(name, "10080")+"/apis/v2/objects/qw-pipeline", 30, name)
		t.Contains(got, "qw-pipeline", "proxied reply")
		t.Logf("  HTTPServer :10080 -> Pipeline -> Proxy -> admin API round-trips")

		t.Docker("restart", name)
		// the published host ports can change on restart
		t.WaitHTTP("http://127.0.0.1:"+t.hostPort(name, "10080")+"/apis/v2/healthz", 60, name)
		t.Logf("smoke test passed (%s, nonroot user: %s, proxy data path, objects kept across restart)", strings.SplitN(v, "\n", 2)[0], u)
	})
}
