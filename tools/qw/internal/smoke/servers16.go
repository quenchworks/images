package smoke

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// sshRemoteForward asks an ssh server to forward bindHost:port back to local, the way
// `ssh -R bindHost:port:local` does. x/crypto's Listen cannot take a hostname bind
// address (sish routes on it), so the tcpip-forward request is sent by hand.
func sshRemoteForward(addr, bindHost string, port uint32, local string) (*ssh.Client, error) {
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := ssh.NewSignerFromKey(key)
	c, err := ssh.Dial("tcp", addr, &ssh.ClientConfig{User: "smoke", Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 10 * time.Second})
	if err != nil {
		return nil, err
	}
	chans := c.HandleChannelOpen("forwarded-tcpip")
	b := make([]byte, 4+len(bindHost)+4)
	binary.BigEndian.PutUint32(b, uint32(len(bindHost)))
	copy(b[4:], bindHost)
	binary.BigEndian.PutUint32(b[4+len(bindHost):], port)
	if ok, _, err := c.SendRequest("tcpip-forward", true, b); err != nil || !ok {
		c.Close()
		return nil, err
	}
	go func() {
		for nc := range chans {
			ch, reqs, err := nc.Accept()
			if err != nil {
				continue
			}
			go ssh.DiscardRequests(reqs)
			go func() {
				defer ch.Close()
				lc, err := net.Dial("tcp", local)
				if err != nil {
					return
				}
				defer lc.Close()
				go io.Copy(lc, ch)
				io.Copy(ch, lc)
			}()
		}
	}()
	return c, nil
}

func init() {
	register("deno", func(t *T) {
		if t.Want == "" {
			t.Fatalf("usage: qw smoke deno <image-ref> <major>")
		}
		ro := []string{"--read-only", "--tmpfs", "/tmp"}
		major := strings.SplitN(t.Want, ".", 2)[0]
		if v, _ := t.RunFlags(ro, "--version"); !strings.HasPrefix(v, "deno "+major+".") {
			t.Fatalf("expected 'deno %s.*', got '%s'", major, v)
		}
		if out, _ := t.RunFlags(ro, "eval", `console.log("quench-" + "deno")`); out != "quench-deno" {
			t.Fatalf("eval output wrong: '%s'", out)
		}
		if uid, _ := t.RunFlags(ro, "eval", "console.log(Deno.uid())"); uid != "1001" {
			t.Fatalf("expected uid 1001, got '%s'", uid)
		}
		t.User("1001")
		w := t.TempDir()
		os.WriteFile(filepath.Join(w, "h.ts"), []byte("const msg: string = \"quench-deno-ts\";\nconsole.log(msg);\n"), 0o644)
		if out, _ := t.RunFlags(append(ro, "-v", w+":/work:ro", "-w", "/work"), "run", "h.ts"); out != "quench-deno-ts" {
			t.Fatalf("TS run output wrong: '%s'", out)
		}
		if _, err := t.RunFlags(ro, "eval", `Deno.writeTextFileSync("/should-fail","x")`); err == nil {
			t.Fatalf("rootfs was writable, expected read-only")
		}
		t.Logf("smoke test passed (Deno %s, read-only rootfs)", major)
	})
	register("flyway", func(t *T) {
		const pw = "quench-smoke-pw"
		u := t.User("1001")
		if v, _ := t.RunFlags([]string{"--read-only", "--tmpfs", "/tmp"}, "-v"); t.Want != "" {
			t.Contains(v, t.Want, "version")
		}
		net_, pg := t.Name("net"), t.Name("pg")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Cleanup(func() { docker("rm", "-f", pg) })
		t.Docker("run", "-d", "--name", pg, "--network", net_, "-e", "POSTGRES_PASSWORD="+pw, "ghcr.io/quenchworks/images/postgresql:18.6")
		psql := func(q string) (string, error) {
			return docker("exec", "-e", "PGPASSWORD="+pw, pg, "psql", "-h", "127.0.0.1", "-U", "postgres", "-tAc", q)
		}
		t.waitFor(60, 2*time.Second, pg, "postgres did not become ready", func() bool { _, err := psql("select 1"); return err == nil })
		w := t.TempDir()
		os.WriteFile(filepath.Join(w, "V1__orders.sql"), []byte("create table orders (id int primary key);\n"), 0o644)
		os.WriteFile(filepath.Join(w, "V2__note.sql"), []byte("alter table orders add column note text;\n"), 0o644)
		fw := func(cmd string) string {
			out, _ := t.RunFlags([]string{"--network", net_, "--read-only", "--tmpfs", "/tmp", "--tmpfs", "/home/nonroot:uid=1001,gid=1001", "-v", w + ":/flyway/sql:ro"},
				"-url=jdbc:postgresql://"+pg+":5432/postgres", "-user=postgres", "-password="+pw, "-locations=filesystem:/flyway/sql", cmd)
			return out
		}
		t.Contains(fw("migrate"), "Successfully applied 2 migrations", "migrate")
		if n := strings.Count(fw("info"), "Success"); n < 2 {
			t.Fatalf("info lists %d successful migrations", n)
		}
		if cols, _ := psql("select string_agg(column_name, ',' order by column_name) from information_schema.columns where table_name='orders'"); cols != "id,note" {
			t.Fatalf("orders columns are '%s'", cols)
		}
		t.Logf("smoke test passed (uid %s, 2 migrations applied to PostgreSQL)", u)
	})
	register("gotenberg", func(t *T) {
		const api = "http://127.0.0.1:13000"
		name, _ := t.httpUp([]string{"-p", "127.0.0.1:13000:3000"}, api+"/health", 60)
		_, v := Get(api + "/version")
		if v == "" || strings.Contains(v, "snapshot") || (t.Want != "" && v != t.Want) {
			t.Fatalf("version '%s', expected %s", v, t.Want)
		}
		_, h := Get(api + "/health")
		t.Contains(h, `"chromium":{"status":"up"`, "chromium up")
		pdf := func(path string, files map[string][]byte) []byte {
			var body strings.Builder
			const bnd = "quenchsmokeboundary"
			for n, b := range files {
				body.WriteString("--" + bnd + "\r\nContent-Disposition: form-data; name=\"files\"; filename=\"" + n + "\"\r\n\r\n")
				body.Write(b)
				body.WriteString("\r\n")
			}
			body.WriteString("--" + bnd + "--\r\n")
			r, err := (&http.Client{Timeout: 60 * time.Second}).Post(api+path, "multipart/form-data; boundary="+bnd, strings.NewReader(body.String()))
			if err != nil {
				t.Fatalf("%s: %v\n%s", path, err, t.Logs(name))
			}
			defer r.Body.Close()
			b, _ := io.ReadAll(r.Body)
			if !strings.HasPrefix(string(b), "%PDF-") {
				t.Fatalf("%s did not return a PDF (%d): %.200s", path, r.StatusCode, b)
			}
			return b
		}
		a := pdf("/forms/chromium/convert/html", map[string][]byte{"index.html": []byte(`<html><body><h1 style="hyphens:auto">QuenchWorks gotenberg smoke</h1></body></html>`)})
		pdf("/forms/pdfengines/merge", map[string][]byte{"a.pdf": a, "b.pdf": a})
		t.Logf("smoke test passed (version %s, nonroot user: %s, HTML to PDF, PDF merge)", v, t.User("1001"))
	})
	register("jetty", func(t *T) {
		w := t.TempDir()
		os.MkdirAll(filepath.Join(w, "hello/WEB-INF"), 0o755)
		mark := t.Name("mark")
		os.WriteFile(filepath.Join(w, "hello/index.html"), []byte(mark+"\n"), 0o644)
		os.WriteFile(filepath.Join(w, "hello/WEB-INF/web.xml"), []byte(`<?xml version="1.0" encoding="UTF-8"?>
<web-app xmlns="https://jakarta.ee/xml/ns/jakartaee" version="6.0"/>
`), 0o644)
		_, body := t.httpUp([]string{"-p", "127.0.0.1:18080:8080", "-v", w + ":/var/lib/jetty/webapps:ro"}, "http://127.0.0.1:18080/hello/", 60)
		if strings.TrimSpace(body) != mark {
			t.Fatalf("unexpected body: %s", body)
		}
		_, h, _ := getH(httpc, "http://127.0.0.1:18080/hello/")
		srv := h.Get("Server")
		t.Match(srv, `^Jetty\(.*\)$`, "Server header")
		if t.Want != "" && srv != "Jetty("+t.Want+")" {
			t.Fatalf("Server %s, expected Jetty(%s)", srv, t.Want)
		}
		if c, _ := Get("http://127.0.0.1:18080/nope/"); c != 404 {
			t.Fatalf("expected 404 for an unknown context, got %d", c)
		}
		t.Logf("smoke test passed (%s, nonroot user: %s, web app deployed and served)", srv, t.User("1001"))
	})
	register("longhorn-engine", func(t *T) {
		u := t.rootLikeUpstream()
		if v := t.RunEntry("longhorn", "version", "--client-only"); t.Want != "" {
			t.Contains(v, `"version": "v`+t.Want+`"`, "version")
		}
		vol := t.Name("vol")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		t.RunFlags([]string{"-v", vol + ":/data", "--entrypoint", "/bin/bash"}, "-c", "cp -p /usr/local/bin/longhorn /data/")
		// the chart copies this binary onto the host; it must run outside our image
		cp, _ := docker("run", "--rm", "-v", vol+":/data", "python:3.12-slim", "/data/longhorn", "version", "--client-only")
		t.Contains(cp, `"clientVersion"`, "copied binary runs off-image")
		name := t.Name("")
		t.Start(name, []string{"--entrypoint", "/bin/bash"}, "-c", `mkdir -p /vol && (longhorn replica /vol --size 64m --listen 0.0.0.0:9502 > /tmp/replica.log 2>&1 &) && sleep 3 &&
exec longhorn controller smoke --frontend "" --size 64m --current-size 64m --replica tcp://localhost:9502 > /tmp/controller.log 2>&1`)
		t.waitFor(30, time.Second, name, "replica never became RW", func() bool {
			out, _ := docker("exec", name, "longhorn", "--url", "localhost:9501", "ls")
			return regexp.MustCompile(`tcp://localhost:9502 *RW`).MatchString(out)
		})
		t.Docker("exec", name, "longhorn", "--url", "localhost:9501", "snapshot", "create")
		t.Match(t.Docker("exec", name, "longhorn", "--url", "localhost:9501", "snapshot", "ls"), `(?m)^[0-9a-f-]{36}$`, "snapshot listed")
		t.RunEntry("tgtd", "--version")
		t.RunEntryAny("sg_raw", "-V")
		if im := t.RunEntry("longhorn-instance-manager", "--version"); t.Want != "" {
			t.Contains(im, "v"+t.Want, "instance-manager version")
		}
		t.Logf("smoke test passed (binary runs off-image, replica+controller snapshot, tgtd, sg_raw; user %q)", u)
	})
	register("php", func(t *T) {
		if t.Want == "" {
			t.Fatalf("usage: qw smoke php <image-ref> <version>")
		}
		ro := []string{"--read-only", "--tmpfs", "/tmp"}
		v, _ := t.RunFlags(ro, "-v")
		if !strings.HasPrefix(v, "PHP "+t.Want+" ") && !strings.HasPrefix(v, "PHP "+t.Want+".") {
			t.Fatalf("expected 'PHP %s', got '%s'", t.Want, v)
		}
		mods, _ := t.RunFlags(ro, "-m")
		for _, e := range strings.Fields("openssl mbstring curl json ctype iconv fileinfo sodium bcmath sockets intl gd zip Phar dom xml xmlreader xmlwriter SimpleXML PDO pdo_sqlite pdo_mysql pdo_pgsql") {
			t.Match(mods, `(?mi)^`+e+`$`, "extension "+e)
		}
		out, _ := t.RunFlags(ro, "-r", `echo "quench-" . "php" . "|" . hash("sha256","x") . "|" . json_encode(["ok"=>true]);`)
		if out != `quench-php|2d711642b726b04401627ca9fbac32f5c8530fb1903cc4db02258717921a4881|{"ok":true}` {
			t.Fatalf("script output wrong: '%s'", out)
		}
		if uid, _ := t.RunFlags(ro, "-r", "echo getmyuid();"); uid != "1001" {
			t.Fatalf("expected uid 1001, got '%s'", uid)
		}
		t.User("1001")
		if _, err := t.RunFlags(ro, "-r", `exit(@file_put_contents("/should-fail","x") === false ? 0 : 1);`); err != nil {
			t.Fatalf("rootfs was writable, expected read-only")
		}
		t.Logf("smoke test passed (PHP %s, read-only rootfs)", t.Want)
	})
	register("sish", func(t *T) {
		u := t.User("1001")
		if v := t.RunAny("--version"); t.Want != "" {
			t.Contains(v, "v"+t.Want, "version")
		}
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:uid=1001,gid=1001", "--tmpfs", "/private:uid=1001,gid=1001", "--tmpfs", "/pubkeys:uid=1001,gid=1001",
			"-p", "127.0.0.1:12222:2222", "-p", "127.0.0.1:18080:8080"},
			"--ssh-address=:2222", "--http-address=:8080", "--domain=sish.test", "--authentication=false",
			"--bind-random-subdomains=false", "--private-keys-directory=/private", "--authentication-keys-directory=/pubkeys")
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		go http.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "quench-tunneled") }))
		t.Cleanup(func() { ln.Close() })
		t.WaitLog(name, "starting SSH service", 30)
		var c *ssh.Client
		t.waitFor(10, time.Second, name, "ssh -R forward refused", func() bool {
			c, err = sshRemoteForward("127.0.0.1:12222", "app", 80, ln.Addr().String())
			return err == nil
		})
		t.Cleanup(func() { c.Close() })
		get := func(host string) string {
			r, _ := http.NewRequest("GET", "http://127.0.0.1:18080/", nil)
			r.Host = host
			resp, err := httpc.Do(r)
			if err != nil {
				return err.Error()
			}
			defer resp.Body.Close()
			b, _ := io.ReadAll(resp.Body)
			return string(b)
		}
		t.waitFor(30, time.Second, name, "tunnel not reached", func() bool { return get("app.sish.test") == "quench-tunneled" })
		if get("other.sish.test") == "quench-tunneled" {
			t.Fatalf("an unknown subdomain reached the tunnel")
		}
		if regexp.MustCompile(`panic|read-only file system`).MatchString(t.Logs(name)) {
			t.Fatalf("crashed or wrote to the root\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed: HTTP tunnel over ssh -R, unknown subdomain refused, user %s", u)
	})
	register("uptime-kuma", func(t *T) {
		u := t.User("1001")
		v := t.RunEntry("/usr/bin/node", "-p", `require("/usr/lib/uptime-kuma/package.json").version`)
		if t.Want != "" && v != t.Want {
			t.Fatalf("uptime-kuma %s, expected %s", v, t.Want)
		}
		vol, name := t.Name("data"), t.Name("")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		t.RunFlags([]string{"-u", "0", "-v", vol + ":/d", "--entrypoint", "/usr/bin/node"}, "-e", `require("fs").chownSync("/d",1001,1001)`)
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp", "-v", vol + ":/app/data", "-p", "127.0.0.1:13001:3001"})
		ep := t.WaitHTTP("http://127.0.0.1:13001/api/entry-page", 120, name)
		t.Contains(ep, `"type":"entryPage"`, "entry-page API")
		_, ui := Get("http://127.0.0.1:13001/")
		t.ContainsFold(ui, `<div id="app"`, "UI served")
		if c, _ := Get("http://127.0.0.1:13001/metrics"); c != 401 {
			t.Fatalf("/metrics returned %d, expected 401", c)
		}
		if out, err := t.RunFlags([]string{"-v", vol + ":/app/data:ro", "--entrypoint", "/usr/bin/node"}, "-e",
			`process.exit(require("fs").statSync("/app/data/kuma.db").size > 0 ? 0 : 1)`); err != nil {
			t.Fatalf("no SQLite database in the data volume: %s", out)
		}
		for _, l := range strings.Split(t.Logs(name), "\n") {
			if regexp.MustCompile(`ERROR|Error:`).MatchString(l) && !strings.Contains(l, "db-config.json is not found") {
				t.Fatalf("server logged errors: %s", l)
			}
		}
		t.Logf("smoke test passed (uptime-kuma %s, uid %s, sqlite in a volume, read-only root)", v, u)
	})
}
