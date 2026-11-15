package smoke

import (
	"net/http"
	"regexp"
	"strings"
	"time"
)

func init() {
	// On a read-only rootfs with /gotosocial/storage on a volume: the server runs the migrations, the admin CLI
	// (inside the running container: it needs the instance application the first start creates) creates,
	// confirms and promotes an account, the instance API shows the version, and after a restart WebFinger
	// still finds the account.
	register("gotosocial", func(t *T) {
		const base = "http://127.0.0.1:18085"
		const host = "gts.quench.test"
		u := t.User("1001")
		t.Contains(t.RunAny("--version"), t.Want, "gotosocial --version")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		flags := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/gotosocial/storage", "-e", "GTS_HOST=" + host}
		start := func(name string) {
			t.Start(name, append(flags, "-p", "127.0.0.1:18085:8080"))
			t.WaitHTTP(base+"/api/v1/instance", 120, name)
		}
		name := t.Name("")
		start(name)
		// the admin CLI needs the instance application the first server start creates
		create := "/gotosocial/gotosocial admin account create --username smoke --email smoke@quench.test --password 'Smoke-Pass-2026-long!' &&" +
			" /gotosocial/gotosocial admin account confirm --username smoke && /gotosocial/gotosocial admin account promote --username smoke"
		if out, err := docker("exec", name, "/bin/sh", "-c", create); err != nil {
			t.Fatalf("admin account create failed: %v\n%s", err, out)
		}
		t.Contains(t.WaitHTTP(base+"/api/v1/instance", 5, name), t.Want, "the instance API version")
		docker("rm", "-f", name)
		start(t.Name("again"))
		wf := t.WaitHTTP(base+"/.well-known/webfinger?resource=acct:smoke@"+host, 10, t.Name("again"))
		if !strings.Contains(wf, "acct:smoke@"+host) {
			t.Fatalf("webfinger after a restart: %q", wf)
		}
		t.Logf("smoke test passed (gotosocial %s: read-only boot, admin created by the CLI, instance API, the account found by WebFinger after a restart; user %s)", t.Want, u)
	})
}

func init() {
	// A wstunnel server restricted to a secret path prefix, our go-httpbin behind it, and a client container
	// forwarding a local port through the tunnel: HTTP through the tunnel reaches go-httpbin, and a client with
	// the wrong prefix gets nothing. All containers on a read-only rootfs.
	register("wstunnel", func(t *T) {
		const via = "http://127.0.0.1:19090"
		u := t.User("1001")
		t.Contains(t.RunAny("--version"), t.Want, "wstunnel --version")
		net := t.Name("net")
		t.Docker("network", "create", net)
		t.Cleanup(func() { docker("network", "rm", net) })
		target := t.Name("httpbin")
		t.Cleanup(func() { docker("rm", "-f", target) })
		t.Docker("run", "-d", "--name", target, "--network", net, "ghcr.io/quenchworks/images/go-httpbin:2.26.0")
		server := t.Name("server")
		t.Start(server, []string{"--read-only", "--network", net}, "server", "ws://0.0.0.0:8080", "--restrict-http-upgrade-path-prefix", "quench-secret")
		client := func(name, prefix string) {
			t.Start(name, []string{"--read-only", "--network", net, "-p", "127.0.0.1:19090:9000"},
				"client", "-L", "tcp://0.0.0.0:9000:"+target+":8080", "--http-upgrade-path-prefix", prefix, "ws://"+server+":8080")
		}
		c := t.Name("client")
		client(c, "quench-secret")
		t.Contains(t.WaitHTTP(via+"/get?quench=tunnel", 60, c), `"quench"`, "go-httpbin through the tunnel")
		docker("rm", "-f", c)
		bad := t.Name("badclient")
		client(bad, "wrong-prefix")
		time.Sleep(3 * time.Second)
		if code, _ := Get(via + "/get"); code == 200 {
			t.Fatalf("a client with the wrong path prefix got through")
		}
		t.Logf("smoke test passed (wstunnel %s: server and client on a read-only rootfs, HTTP tunneled to go-httpbin, a wrong path prefix refused; user %s)", t.Want, u)
	})
}

// wfAPI calls the Write.as-compatible API with an optional token and returns the status and body.
func wfAPI(method, url, token, body string) (int, string) {
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Token "+token)
	}
	return insecureDo(req)
}

var wfField = regexp.MustCompile(`"(access_token|id)":"([^"]+)"`)

func wfGet(body, key string) string {
	for _, m := range wfField.FindAllStringSubmatch(body, -1) {
		if m[1] == key {
			return m[2]
		}
	}
	return ""
}

func init() {
	// On a read-only rootfs with /data on a volume: the start script initialises the database and keys, the
	// CLI creates an admin, the API refuses a wrong password, the admin publishes a post, and after a restart
	// the post reads back with a new login.
	register("writefreely", func(t *T) {
		const base = "http://127.0.0.1:18086"
		const pw = "Smoke-Pass-2026"
		u := t.User("1001")
		t.Contains(t.RunAny("--version"), t.Want, "writefreely --version")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		start := func(name string) {
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/data", "-p", "127.0.0.1:18086:8080"})
			t.WaitHTTP(base+"/", 90, name)
		}
		name := t.Name("")
		start(name)
		if out, err := docker("exec", name, "/usr/bin/writefreely", "-c", "/etc/writefreely/config.ini", "user", "create", "--admin", "smokeadmin:"+pw); err != nil {
			t.Fatalf("admin user create failed: %v\n%s", err, out)
		}
		login := func(p string) (int, string) {
			return wfAPI("POST", base+"/api/auth/login", "", `{"alias":"smokeadmin","pass":"`+p+`"}`)
		}
		code, body := login(pw)
		tok := wfGet(body, "access_token")
		if code != 200 || tok == "" {
			t.Fatalf("login: %d %s", code, body)
		}
		code, body = wfAPI("POST", base+"/api/posts", tok, `{"body":"quench smoke post","title":"Smoke"}`)
		id := wfGet(body, "id")
		if code/100 != 2 || id == "" {
			t.Fatalf("publish: %d %s", code, body)
		}
		// WriteFreely rate-limits logins (429), so the wrong password is tried last, before the restart resets it
		if code, _ := login("wrong-password-1"); code == 200 {
			t.Fatalf("a wrong password logged in")
		}
		docker("rm", "-f", name)
		start(t.Name("again"))
		_, body = login(pw)
		if tok = wfGet(body, "access_token"); tok == "" {
			t.Fatalf("login after a restart: %s", body)
		}
		if code, body = wfAPI("GET", base+"/api/posts/"+id, tok, ""); code != 200 || !strings.Contains(body, "quench smoke post") {
			t.Fatalf("the post after a restart: %d %s", code, body)
		}
		t.Logf("smoke test passed (writefreely %s: read-only boot, database and keys initialised, admin created, wrong password refused, a post published and read back after a restart; user %s)", t.Want, u)
	})
}
