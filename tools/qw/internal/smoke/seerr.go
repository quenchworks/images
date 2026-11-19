package smoke

import (
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"
)

// seerrFakeJellyfin is a Jellyfin stand-in, run with the image's own node, answering the four calls Seerr's first-run
// setup makes: sign-in (admin / FakeJellyfin-2026 only), API key create + list, and the public server info.
const seerrFakeJellyfin = `
const http = require("http");
const user = { Id: "a1b2c3d4e5f6", Name: "admin", ServerId: "fake-server-1", Policy: { IsAdministrator: true } };
const send = (res, code, body) => { res.writeHead(code, { "Content-Type": "application/json" }); res.end(body === undefined ? "" : JSON.stringify(body)); };
http.createServer((req, res) => {
  let b = "";
  req.on("data", (c) => (b += c));
  req.on("end", () => {
    const p = req.url.split("?")[0];
    console.log(req.method, p);
    if (req.method === "POST" && p === "/Users/AuthenticateByName") {
      let j = {}; try { j = JSON.parse(b); } catch {}
      if (j.Username === "admin" && j.Pw === "FakeJellyfin-2026") return send(res, 200, { User: user, AccessToken: "fake-user-token" });
      return send(res, 401);
    }
    if (p === "/Auth/Keys") return req.method === "POST" ? send(res, 204) : send(res, 200, { Items: [{ AppName: "Seerr", AccessToken: "fake-api-key" }] });
    if (p === "/System/Info/Public") return send(res, 200, { ServerName: "Fake Jellyfin", Id: "fake-server-1", Version: "10.11.0" });
    if (p === "/System/Info") return send(res, 200, { ServerName: "Fake Jellyfin", Id: "fake-server-1", Version: "10.11.0" });
    if (p.startsWith("/Users/")) return send(res, 200, user);
    send(res, 200, { Items: [], TotalRecordCount: 0 });
  });
}).listen(8096, () => console.log("fake jellyfin on 8096"));
`

func init() {
	// On a read-only rootfs with /app/config on a volume: TypeORM's migrations run at start (through the floored
	// glob 14); first-run setup signs the admin in through a fake Jellyfin (a wrong Jellyfin password is refused),
	// the setup is finished, and after a restart the admin signs in again with the stored server settings.
	register("seerr", func(t *T) {
		const base = "http://127.0.0.1:18091"
		u := t.User("1001")
		net := t.Name("net")
		t.Docker("network", "create", net)
		t.Cleanup(func() { docker("network", "rm", net) })
		jf := t.Name("jellyfin")
		t.Docker("run", "-d", "--name", jf, "--network", net, "--network-alias", "jellyfin", "--entrypoint", "/usr/bin/node", t.Image, "-e", seerrFakeJellyfin)
		t.Cleanup(func() { docker("rm", "-f", jf) })
		vol := t.Name("config")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		flags := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--tmpfs", "/app/.next/cache:rw,uid=1001,gid=1001",
			"--network", net, "-v", vol + ":/app/config", "-p", "127.0.0.1:18091:5055"}
		start := func(name string) {
			t.Start(name, flags)
			for i := 0; i < 90; i++ {
				if c, _ := restDo("GET", base+"/api/v1/status", "", "", ""); c == http.StatusOK {
					return
				}
				time.Sleep(2 * time.Second)
			}
			t.Fatalf("seerr did not answer /api/v1/status:\n%s", t.Logs(name))
		}
		jar, _ := cookiejar.New(nil)
		cl := &http.Client{Jar: jar, Timeout: 60 * time.Second}
		do := func(method, path, body string) (int, string) {
			req, _ := http.NewRequest(method, base+path, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r, err := cl.Do(req)
			if err != nil {
				return 0, err.Error()
			}
			defer r.Body.Close()
			buf := new(strings.Builder)
			b := make([]byte, 32<<10)
			for {
				n, err := r.Body.Read(b)
				buf.Write(b[:n])
				if err != nil {
					break
				}
			}
			return r.StatusCode, buf.String()
		}
		name := t.Name("")
		start(name)
		_, st := restDo("GET", base+"/api/v1/status", "", "", "")
		t.Contains(st, `"version":"`+t.Want+`"`, "/api/v1/status")
		_, pub := restDo("GET", base+"/api/v1/settings/public", "", "", "")
		t.Contains(pub, `"initialized":false`, "public settings before setup")
		setup := func(pw string) (int, string) {
			return do("POST", "/api/v1/auth/jellyfin", `{"username":"admin","password":"`+pw+`","hostname":"jellyfin","port":8096,"useSsl":false,"urlBase":"","email":"admin@quench-works.com","serverType":2}`)
		}
		if c, _ := setup("wrong-password-1"); c == http.StatusOK {
			t.Fatalf("a wrong Jellyfin password was accepted")
		}
		if c, b := setup("FakeJellyfin-2026"); c != http.StatusOK {
			t.Fatalf("setup sign-in through Jellyfin: %d %s\n%s\n--- fake jellyfin:\n%s", c, b, t.Logs(name), t.Logs(jf))
		}
		c, me := do("GET", "/api/v1/auth/me", "")
		if c != http.StatusOK {
			t.Fatalf("/auth/me after sign-in: %d %s", c, me)
		}
		t.Contains(me, `"jellyfinUsername":"admin"`, "/auth/me")
		if c, b := do("POST", "/api/v1/settings/initialize", ""); c != http.StatusOK {
			t.Fatalf("finish setup: %d %s", c, b)
		}
		docker("rm", "-f", name)
		start(t.Name("again"))
		_, pub = restDo("GET", base+"/api/v1/settings/public", "", "", "")
		t.Contains(pub, `"initialized":true`, "public settings after a restart")
		jar2, _ := cookiejar.New(nil)
		cl.Jar = jar2
		if c, b := do("POST", "/api/v1/auth/jellyfin", `{"username":"admin","password":"FakeJellyfin-2026"}`); c != http.StatusOK {
			t.Fatalf("admin sign-in after a restart: %d %s", c, b)
		}
		if c, me := do("GET", "/api/v1/auth/me", ""); c != http.StatusOK || !strings.Contains(me, `"jellyfinUsername":"admin"`) {
			t.Fatalf("/auth/me after a restart: %d %s", c, me)
		}
		t.Logf("smoke test passed (seerr %s: read-only boot through the TypeORM migrations, setup signs the admin in through a fake Jellyfin, a wrong Jellyfin password refused, setup finished, admin signs in again after a restart; user %s)", t.Want, u)
	})
}
