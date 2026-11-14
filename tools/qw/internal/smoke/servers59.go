package smoke

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

func init() {
	// Drives nginx-proxy-manager end to end on a read-only root as uid 1001: the first admin from
	// INITIAL_ADMIN_*, /api/ reports OK and the release, an API login creates a proxy host
	// "smoke.test" forwarding to the admin server (the backend renders it, runs nginx -t and
	// reloads), and a request on the HTTP port with Host: smoke.test comes back through it.
	register("nginx-proxy-manager", func(t *T) {
		u := t.User("1001")
		data, le, name := t.Name("data"), t.Name("le"), t.Name("")
		for _, v := range []string{data, le} {
			t.Docker("volume", "create", v)
			vol := v
			t.Cleanup(func() { docker("volume", "rm", vol) })
			// a fresh named volume is root-owned; hand it to uid 1001 the way a pod fsGroup would
			t.Docker("run", "--rm", "-u", "0", "-v", v+":/d", "--entrypoint", "/usr/bin/node", t.Image, "-e", `require("fs").chownSync("/d",1001,1001)`)
		}
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp", "-v", data + ":/data", "-v", le + ":/etc/letsencrypt",
			"-e", "INITIAL_ADMIN_EMAIL=admin@smoke.test", "-e", "INITIAL_ADMIN_PASSWORD=quench-smoke-pw", "-e", "IP_RANGES_FETCH_ENABLED=false",
			"-p", "127.0.0.1::8181", "-p", "127.0.0.1::8080"})
		admin := "http://127.0.0.1:" + t.hostPort(name, "8181")
		web := "http://127.0.0.1:" + t.hostPort(name, "8080")
		var api string
		t.waitFor(120, time.Second, name, "API never reported OK", func() bool {
			_, api = Get(admin + "/api/")
			return strings.Contains(api, `"status":"OK"`)
		})
		t.Logf("api: %s", api)
		if t.Want != "" {
			p := strings.SplitN(t.Want, ".", 3)
			if len(p) == 3 {
				t.Contains(api, fmt.Sprintf(`"major":%s,"minor":%s,"revision":%s`, p[0], p[1], p[2]), "release version")
			}
		}
		js := map[string]string{"Content-Type": "application/json"}
		_, tb := DoH("POST", admin+"/api/tokens", `{"identity":"admin@smoke.test","secret":"quench-smoke-pw"}`, js)
		var tok struct{ Token string }
		if json.Unmarshal([]byte(tb), &tok); tok.Token == "" {
			t.Fatalf("no token: %.300s", tb)
		}
		auth := map[string]string{"Content-Type": "application/json", "Authorization": "Bearer " + tok.Token}
		_, cb := DoH("POST", admin+"/api/nginx/proxy-hosts",
			`{"domain_names":["smoke.test"],"forward_scheme":"http","forward_host":"127.0.0.1","forward_port":8181,"meta":{}}`, auth)
		var host struct{ ID int }
		if json.Unmarshal([]byte(cb), &host); host.ID == 0 {
			t.Fatalf("proxy host not created: %.300s", cb)
		}
		// the backend writes the config, tests it and reloads nginx after answering; it records
		// the outcome in the host's meta
		t.waitFor(30, time.Second, name, "proxy host not online and serving", func() bool {
			_, h := DoH("GET", fmt.Sprintf("%s/api/nginx/proxy-hosts/%d", admin, host.ID), "", auth)
			_, body := DoH("GET", web+"/", "", map[string]string{"Host": "smoke.test"})
			return strings.Contains(h, `"nginx_online":true`) && strings.Contains(strings.ToLower(body), `<div id="root"`)
		})
		t.Logf("Host: smoke.test on :8080 served the admin UI through the new proxy host")
		t.Logf("%s", t.Docker("exec", name, "certbot", "--version"))
		if m := regexp.MustCompile(`(?i)read-only file system|permission denied|EACCES|\[emerg\]`).FindString(t.Logs(name)); m != "" {
			t.Fatalf("errors in the logs (%s):\n%s", m, t.Logs(name))
		}
		t.Logf("smoke test passed (nonroot %s, read-only boot, API login, proxy host created and serving)", u)
	})
}
