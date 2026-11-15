package smoke

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

// gokapiSetup is the first-start wizard as the browser submits it: every field, SQLite in the
// data dir, internal (username and password) login, local storage and no end-to-end encryption.
func gokapiSetup(base, user, pw string) string {
	f := map[string]string{
		"dbtype_sel": "0", "sqlite_location": "/app/data/gokapi.sqlite",
		"redis_location": "", "redis_prefix": "", "redis_user": "", "redis_password": "", "redis_ssl_sel": "0",
		"authentication_sel": "0", "auth_username": user, "auth_pw": pw,
		"oauth_provider": "", "oauth_id": "", "oauth_secret": "", "oauth_admin_user": "", "oauth_scope_groups": "",
		"oauth_allowed_groups": "", "oauth_restrict_groups": "0", "oauth_only_registered_users": "0", "oauth_recheck_interval": "12",
		"auth_headerkey": "", "auth_header_admin": "", "auth_header_only_registered_users": "0",
		"port": "53842", "localhost_sel": "0", "ssl_sel": "0", "logip_sel": "0", "showfilename_sel": "0",
		"url": base + "/", "url_redirection": base + "/", "public_name": "Gokapi",
		"encrypt_sel": "0", "enc_pw": "", "cleare2e": "0",
		"storage_sel": "0", "storage_sel_image": "0", "storage_sel_proxy": "0",
		"s3_bucket": "", "s3_region": "", "s3_api": "", "s3_secret": "", "s3_endpoint": "",
	}
	var arr []map[string]string
	for k, v := range f {
		arr = append(arr, map[string]string{"name": k, "value": v})
	}
	b, _ := json.Marshal(arr)
	return string(b)
}

func init() {
	// On a read-only rootfs with config and data volumes: the first start serves the setup wizard,
	// the wizard's JSON form configures SQLite and an admin login, the server comes up on the same
	// port, a wrong password is refused, the admin logs in and reaches /admin, and after a restart
	// on the same volumes the configuration is kept (the login page answers, no wizard).
	register("gokapi", func(t *T) {
		const base = "http://127.0.0.1:15384"
		const pw = "Smoke-Pass-2026"
		u := t.User("1001")
		t.Contains(t.RunAny("--version"), t.Want, "gokapi --version")
		cfg, data := t.Name("config"), t.Name("data")
		for _, v := range []string{cfg, data} {
			v := v
			t.Docker("volume", "create", v)
			t.Cleanup(func() { docker("volume", "rm", v) })
		}
		start := func(name, wait string) {
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", cfg + ":/app/config", "-v", data + ":/app/data",
				"-p", "127.0.0.1:15384:53842"})
			t.waitFor(60, time.Second, name, "gokapi never answered "+wait, func() bool {
				if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
					t.Fatalf("gokapi exited\n%s", t.Logs(name))
				}
				c, b := Get(base + wait)
				// the setup server answers every path with 200; after the setup only the real server has a login form
				return c == 200 && (wait != "/login" || strings.Contains(b, "csrf-token"))
			})
		}
		name := t.Name("")
		start(name, "/setup/start")
		if c, b := DoH("POST", base+"/setup/setupResult", gokapiSetup(base, "admin", pw), map[string]string{"Content-Type": "application/json"}); c != 200 || !strings.Contains(b, "OK") {
			t.Fatalf("setup answered %d %s\n%s", c, b, t.Logs(name))
		}
		// the setup server shuts down and the real one starts on the same port; until then every
		// path answers 200 with a maintenance page, so wait for the login form itself
		t.waitFor(60, time.Second, name, "gokapi never served its login form after the setup", func() bool {
			c, b := Get(base + "/login")
			return c == 200 && strings.Contains(b, "csrf-token")
		})
		jar, _ := cookiejar.New(nil)
		cl := &http.Client{Jar: jar, Timeout: 15 * time.Second}
		login := func(p string) string {
			resp, err := cl.Get(base + "/login")
			if err != nil {
				t.Fatalf("login page: %v", err)
			}
			page, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			tok := t.Match(string(page), `name="csrf-token" value="([^"]+)"`, "CSRF token on the login page")
			resp, err = cl.PostForm(base+"/login", url.Values{"username": {"admin"}, "password": {p}, "csrf-token": {tok}})
			if err != nil {
				t.Fatalf("login: %v", err)
			}
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return string(b)
		}
		// success answers a meta refresh to ./admin; a failure renders the form again
		if strings.Contains(login("wrong-password-1"), "URL=./admin") {
			t.Fatalf("a wrong password logged in")
		}
		if b := login(pw); !strings.Contains(b, "URL=./admin") {
			t.Fatalf("login did not redirect to admin\n%s", t.Logs(name))
		}
		resp, err := cl.Get(base + "/admin")
		if err != nil || resp.StatusCode != 200 {
			t.Fatalf("admin page: %v %v", err, resp)
		}
		page, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Contains(string(page), "logout", "admin page after login")
		docker("rm", "-f", name)
		again := t.Name("again")
		start(again, "/login")
		if c, _ := Get(base + "/setup/start"); c == 200 {
			t.Fatalf("the setup wizard came back after a restart")
		}
		t.Logf("smoke test passed (gokapi %s: read-only boot, setup wizard via its JSON form, wrong password refused, admin login, config kept across a restart; user %s)", t.Want, u)
	})
}
