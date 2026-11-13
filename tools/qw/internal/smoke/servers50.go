package smoke

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func init() {
	register("dependency-track", func(t *T) {
		const pgImg = "ghcr.io/quenchworks/images/postgresql@sha256:4c0580fd2968b8d6e8fb008e0eb74b55c54464b041c235842e4d1cc37ed9a445"
		const api = "http://127.0.0.1:18080/api"
		net_, pg, app, pw := t.Name("net"), t.Name("pg"), t.Name(""), t.Name("pgpw")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		// The postgresql image creates POSTGRES_DB only when it differs from the superuser name.
		t.Docker("run", "-d", "--name", pg, "--network", net_, "--network-alias", "pg", "-e", "POSTGRES_PASSWORD="+pw, "-e", "POSTGRES_DB=dtrack", pgImg)
		t.Cleanup(func() { docker("rm", "-f", pg) })
		// The server exits instead of retrying its database at startup.
		t.waitFor(60, time.Second, pg, "postgres never ready", func() bool {
			_, err := docker("exec", pg, "pg_isready", "-h", "127.0.0.1", "-U", "postgres", "-d", "dtrack")
			return err == nil
		})
		t.Start(app, []string{"--network", net_, "-p", "127.0.0.1:18080:8080", "-p", "127.0.0.1:19000:9000",
			"-e", "DT_DATASOURCE_URL=jdbc:postgresql://pg:5432/dtrack", "-e", "DT_DATASOURCE_USERNAME=postgres", "-e", "DT_DATASOURCE_PASSWORD=" + pw})
		t.waitFor(90, 2*time.Second, app, "never healthy", func() bool {
			_, h := Get("http://127.0.0.1:19000/health")
			return regexp.MustCompile(`"status" *: *"UP"`).MatchString(h)
		})
		t.Logf("  /health: UP")
		// /health can turn UP a moment before the API port serves.
		var v struct{ Version string }
		t.waitFor(30, 2*time.Second, app, "no version", func() bool {
			_, b := Get(api + "/version")
			return json.Unmarshal([]byte(b), &v) == nil && v.Version != ""
		})
		t.Logf("reported version: %s", v.Version)
		if t.Want != "" && v.Version != t.Want {
			t.Fatalf("expected %s", t.Want)
		}
		newpw := "Quench-" + hexRand(4) + "-Works!"
		form := "application/x-www-form-urlencoded"
		if c, b := Post(api+"/v1/user/forceChangePassword", form, url.Values{"username": {"admin"}, "password": {"admin"},
			"newPassword": {newpw}, "confirmPassword": {newpw}}.Encode()); c/100 != 2 {
			t.Fatalf("forced password change failed: %d %s", c, b)
		}
		c, token := Post(api+"/v1/user/login", form, url.Values{"username": {"admin"}, "password": {newpw}}.Encode())
		if c != 200 || token == "" {
			t.Fatalf("login failed: %d %s", c, token)
		}
		t.Logf("  admin changed the forced password and logged in")
		h := map[string]string{"Authorization": "Bearer " + token, "Content-Type": "application/json"}
		if c, b := DoH("PUT", api+"/v1/project", `{"name":"quench-smoke","version":"1.0.0"}`, h); c/100 != 2 {
			t.Fatalf("project create failed: %d %s", c, b)
		}
		_, p := DoH("GET", api+"/v1/project?name=quench-smoke", "", h)
		t.Match(p, `"name" *: *"quench-smoke"`, "project listed")
		t.Logf("  created and read back project quench-smoke")
		t.Logf("smoke test passed (version %s, nonroot user: %s, PostgreSQL, admin login, project create)", v.Version, t.User("1001"))
	})
	register("syncthing", func(t *T) {
		user := t.User("1001")
		v := t.RunEntry("/usr/bin/syncthing", "--version")
		t.Logf("%s", strings.TrimSpace(v))
		if t.Want != "" {
			t.Contains(v, "v"+t.Want+" ", "version")
		}
		net_, key := t.Name("net"), t.Name("key")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		ports := map[string]string{"a": "18384", "b": "18385"}
		names := map[string]string{}
		for _, n := range []string{"a", "b"} {
			names[n] = t.Name(n)
			t.Cleanup(func() { docker("volume", "rm", names[n]) })
			t.Start(names[n], []string{"--network", net_, "--network-alias", n, "--read-only", "--tmpfs", "/tmp:uid=1001,gid=1001",
				"-v", names[n] + ":/var/syncthing", "-e", "STGUIAPIKEY=" + key, "-p", "127.0.0.1:" + ports[n] + ":8384"})
		}
		rest := func(n, method, path, body string) (int, string) {
			return DoH(method, "http://127.0.0.1:"+ports[n]+"/rest/"+path, body, map[string]string{"X-API-Key": key, "Content-Type": "application/json"})
		}
		ids := map[string]string{}
		for _, n := range []string{"a", "b"} {
			t.waitFor(60, time.Second, names[n], "REST never up on "+n, func() bool {
				var s struct{ MyID string }
				_, b := rest(n, "GET", "system/status", "")
				json.Unmarshal([]byte(b), &s)
				ids[n] = s.MyID
				return s.MyID != ""
			})
		}
		t.Logf("  devices: %s / %s", ids["a"], ids["b"])
		if c, _ := Get("http://127.0.0.1:18384/rest/system/status"); c != 401 && c != 403 {
			t.Fatalf("REST without the key got HTTP %d", c)
		}
		t.Logf("  REST without the API key refused")
		// Each side knows the other at a fixed address; one shared folder on both.
		for n, peer := range map[string]string{"a": "b", "b": "a"} {
			if c, b := rest(n, "POST", "config/devices", `{"deviceID":"`+ids[peer]+`","name":"`+peer+`","addresses":["tcp://`+peer+`:22000"]}`); c/100 != 2 {
				t.Fatalf("add device on %s: %d %s", n, c, b)
			}
			if c, b := rest(n, "POST", "config/folders", `{"id":"smoke","label":"smoke","path":"/var/syncthing/smoke","rescanIntervalS":5,"devices":[{"deviceID":"`+ids["a"]+`"},{"deviceID":"`+ids["b"]+`"}]}`); c/100 != 2 {
				t.Fatalf("add folder on %s: %d %s", n, c, b)
			}
		}
		dir := t.TempDir()
		want := "quench-synced-" + hexRand(4)
		src := filepath.Join(dir, "hello.txt")
		os.WriteFile(src, []byte(want+"\n"), 0o644)
		t.waitFor(20, time.Second, names["a"], "copy into A failed", func() bool {
			_, err := docker("cp", src, names["a"]+":/var/syncthing/smoke/hello.txt")
			return err == nil
		})
		rest("a", "POST", "db/scan?folder=smoke", "")
		out := filepath.Join(dir, "got")
		t.waitFor(90, time.Second, names["b"], "file never reached B\n"+t.Logs(names["a"]), func() bool {
			os.RemoveAll(out)
			if _, err := docker("cp", names["b"]+":/var/syncthing/smoke/hello.txt", out); err != nil {
				return false
			}
			got, _ := os.ReadFile(out)
			return strings.TrimSpace(string(got)) == want
		})
		t.Logf("  file written on A arrived on B: %s", want)
		if m := regexp.MustCompile(`(?i)read-only file system|panic`).FindString(t.Logs(names["a"])); m != "" {
			t.Fatalf("crashed or wrote to the root (%s)", m)
		}
		t.Logf("smoke test passed (syncthing %s: two devices paired over REST, folder synced, API key enforced, read-only root; user %s)", t.Want, user)
	})
	register("blocky", func(t *T) {
		t.User("1001")
		v := t.RunAny("version")
		t.Logf("%s", strings.TrimSpace(v))
		t.WantVersion(v)
		dir := t.TempDir()
		cfg := filepath.Join(dir, "config.yml")
		os.WriteFile(cfg, []byte(`upstreams:
  init:
    strategy: fast
  groups:
    default:
      - 1.1.1.1
customDNS:
  mapping:
    smoke.quench.test: 192.0.2.10
blocking:
  denylists:
    ads:
      - |
        blocked.quench.test
  clientGroupsBlock:
    default:
      - ads
ports:
  dns: 5353
  http: 4000
prometheus:
  enable: true
`), 0o644)
		name := t.Name("")
		t.Start(name, []string{"--read-only", "-v", cfg + ":/app/config.yml:ro", "-p", "127.0.0.1:14000:4000"})
		q := func(host string) string {
			out, _ := docker("exec", name, "/usr/bin/blocky", "query", host, "--apiPort", "4000")
			return out
		}
		t.waitFor(30, time.Second, name, "custom mapping not answered", func() bool { return strings.Contains(q("smoke.quench.test"), "192.0.2.10") })
		t.Logf("custom DNS: smoke.quench.test -> 192.0.2.10")
		t.ContainsFold(q("blocked.quench.test"), "BLOCKED", "denylisted name blocked")
		t.Logf("denylist: blocked.quench.test blocked")
		_, m := Get("http://127.0.0.1:14000/metrics")
		t.Contains(m, "blocky_", "metrics")
		if m := regexp.MustCompile(`(?i)panic|read-only file system`).FindString(t.Logs(name)); m != "" {
			t.Fatalf("crashed or wrote to the root (%s)", m)
		}
		t.Logf("smoke test passed: custom DNS, denylist blocking, metrics, user 1001")
	})
}
