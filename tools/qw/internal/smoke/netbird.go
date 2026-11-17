package smoke

import (
	"net/http"
	"strings"
)

// NetBird images (#108). Version checks where the binary has one; signal, relay and upload also
// start and serve. Management, the combined server, the proxy and router peers are tested
// end to end by the netbird chart's kind gate.
func init() {
	register("netbird-management", func(t *T) {
		t.WantVersion(t.RunEntry("/usr/bin/netbird-mgmt", "--version"))
		t.Logf("smoke test passed (netbird-management %s: version; user %s)", t.Want, t.User("1001"))
	})
	register("netbird-signal", func(t *T) {
		t.WantVersion(t.RunEntry("/usr/bin/netbird-signal", "--version"))
		name := t.Name("")
		t.Start(name, []string{"--read-only", "-p", "127.0.0.1:18120:10000"}, "run", "--port", "10000", "--log-file", "console")
		t.WaitTCP(60, name, "127.0.0.1:18120")
		t.Logf("smoke test passed (netbird-signal %s: version, gRPC port up on a read-only rootfs; user %s)", t.Want, t.User("1001"))
	})
	register("netbird-relay", func(t *T) {
		name := t.Name("")
		t.Start(name, []string{"--read-only", "-p", "127.0.0.1:18121:9000"},
			"--listen-address", ":33080", "--exposed-address", "127.0.0.1:33080", "--auth-secret", "quench-smoke-secret")
		// the relay has no version command and does not log one: health plus the WS listener
		body := t.WaitHTTP("http://127.0.0.1:18121/health", 60, name)
		t.Contains(t.Logs(name), "WS server listening address: :33080", "the relay WS listener")
		t.Logf("smoke test passed (netbird-relay %s: health %q on a read-only rootfs; user %s)", t.Want, strings.TrimSpace(body), t.User("1001"))
	})
	register("netbird-server", func(t *T) {
		out := t.RunEntryAny("/usr/bin/netbird-server", "--help")
		t.Contains(out, "Combined Netbird server", "the combined server did not run")
		t.Logf("smoke test passed (netbird-server %s: runs; user %s)", t.Want, t.User("1001"))
	})
	register("netbird-upload", func(t *T) {
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-p", "127.0.0.1:18122:8080"})
		t.WaitTCP(60, name, "127.0.0.1:18122")
		t.Logf("smoke test passed (netbird-upload %s: listening on a read-only rootfs; user %s)", t.Want, t.User("1001"))
	})
	register("netbird-reverse-proxy", func(t *T) {
		t.Contains(t.RunEntryAny("/usr/bin/netbird-proxy", "--version"), "Version: "+t.Want, "version not stamped")
		t.Logf("smoke test passed (netbird-reverse-proxy %s: version; user %s)", t.Want, t.User("1001"))
	})
	register("netbird-nblink", func(t *T) {
		t.WantVersion(t.RunEntry("/usr/bin/nblink", "--version"))
		t.Logf("smoke test passed (netbird-nblink %s: version; user %s)", t.Want, t.User("1001"))
	})
	register("netbird", func(t *T) {
		t.WantVersion(t.RunEntry("/usr/bin/netbird", "version"))
		t.Logf("smoke test passed (netbird %s: version; user %s)", t.Want, t.User("1001"))
	})
	// the init script writes the OIDC settings and the CSP into a /tmp copy; nginx serves it on 8080
	register("netbird-dashboard", func(t *T) {
		const base = "http://127.0.0.1:18123"
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-p", "127.0.0.1:18123:8080",
			"-e", "AUTH_AUTHORITY=https://idp.quench.test/oauth2", "-e", "AUTH_CLIENT_ID=netbird-dashboard",
			"-e", "AUTH_AUDIENCE=netbird", "-e", "AUTH_SUPPORTED_SCOPES=openid profile email",
			"-e", "USE_AUTH0=false", "-e", "NETBIRD_MGMT_API_ENDPOINT=https://netbird.quench.test"})
		page := t.WaitHTTP(base+"/", 90, name)
		t.ContainsFold(page, "<html", "the dashboard page")
		// access tokens may only go to the management API
		if c, b := Get(base + "/OidcTrustedDomains.js"); c != http.StatusOK || !strings.Contains(b, "https://netbird.quench.test") {
			t.Fatalf("OidcTrustedDomains.js does not carry the management endpoint: %d\n%.400s\n%s", c, b, t.Logs(name))
		}
		r, err := http.Get(base + "/")
		if csp := r.Header.Get("Content-Security-Policy"); err != nil || !strings.Contains(csp, "netbird.quench.test") || !strings.Contains(csp, "idp.quench.test") {
			t.Fatalf("the CSP header lacks the management endpoint or the IdP: %v %q", err, csp)
		}
		r.Body.Close()
		t.Logf("smoke test passed (netbird-dashboard %s: read-only rootfs, OIDC settings and CSP written at start, served on 8080; user %s)", t.Want, t.User("1001"))
	})
}
