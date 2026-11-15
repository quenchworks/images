package smoke

import "time"

func init() {
	// On a read-only rootfs with tmpfs data: /healthz, OIDC discovery naming APP_URL as the
	// issuer, the embedded SPA, and the admin API through STATIC_API_KEY creating a user and an
	// OIDC client that read back.
	register("pocket-id", func(t *T) {
		const base = "http://127.0.0.1:11411"
		const appURL = "http://localhost:11411"
		const key = "smoke-static-api-key-0123456789"
		u := t.User("1001")
		t.Contains(t.RunAny("version"), t.Want, "pocket-id version")
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--tmpfs", "/app/data:rw,uid=1001,gid=1001", "-p", "127.0.0.1:11411:1411",
			"-e", "ENCRYPTION_KEY=smoke-encryption-key-0123456789", "-e", "APP_URL=" + appURL, "-e", "STATIC_API_KEY=" + key,
			"-e", "ANALYTICS_DISABLED=true", "-e", "VERSION_CHECK_DISABLED=true"})
		t.waitFor(60, time.Second, name, "pocket-id never answered /healthz", func() bool {
			if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
				t.Fatalf("pocket-id exited\n%s", t.Logs(name))
			}
			c, _ := Get(base + "/healthz") // 204 No Content when healthy
			return c == 204 || c == 200
		})
		_, disc := Get(base + "/.well-known/openid-configuration")
		t.Contains(disc, `"issuer":"`+appURL+`"`, "OIDC discovery issuer")
		_, spa := Get(base + "/")
		t.Contains(spa, "/_app/immutable/", "embedded SvelteKit SPA")
		api := map[string]string{"X-API-KEY": key, "Content-Type": "application/json"}
		if c, b := DoH("POST", base+"/api/users", `{"username":"smoke","email":"smoke@example.com","firstName":"Smoke","lastName":"Test","isAdmin":false}`, api); c >= 300 {
			t.Fatalf("create user through the static API key: %d %s", c, b)
		}
		if c, b := DoH("POST", base+"/api/oidc/clients", `{"name":"smoke-app","callbackURLs":["https://app.example.com/callback"],"isPublic":false,"pkceEnabled":true}`, api); c >= 300 {
			t.Fatalf("create OIDC client: %d %s", c, b)
		}
		_, users := DoH("GET", base+"/api/users", "", api)
		t.Contains(users, `"smoke@example.com"`, "user listed")
		_, clients := DoH("GET", base+"/api/oidc/clients", "", api)
		t.Contains(clients, `"smoke-app"`, "OIDC client listed")
		if c, _ := DoH("GET", base+"/api/users", "", map[string]string{"X-API-KEY": "wrong-key"}); c < 400 {
			t.Fatalf("a wrong API key answered %d", c)
		}
		t.Logf("smoke test passed (pocket-id %s: read-only boot, /healthz, OIDC discovery, SPA, user + OIDC client through the API key, wrong key refused; user %s)", t.Want, u)
	})
}
