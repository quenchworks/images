package smoke

import (
	"context"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func init() {
	// On a read-only rootfs with a seeded config (bcrypt admin, DNS on 5353, one block rule) on a
	// writable volume: a DNS query for the blocked name answers 0.0.0.0, the admin logs in and reads
	// /control/status, and a wrong password is refused.
	register("adguardhome", func(t *T) {
		const pw = "Smoke-Admin-2026"
		u := t.User("1001")
		t.Contains(t.RunAny("--version"), "v"+t.Want, "AdGuardHome --version")
		dir, err := os.MkdirTemp(envOr("TMPDIR", ""), "adguard-smoke-")
		if err != nil {
			t.Fatalf("temp dir: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(dir) })
		_ = os.Chmod(dir, 0o777) // AdGuard (uid 1001) writes its config back
		hash, _ := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
		cfg := `http:
  address: 0.0.0.0:3000
users:
  - name: admin
    password: ` + string(hash) + `
dns:
  bind_hosts: [0.0.0.0]
  port: 5353
  upstream_dns: ["127.0.0.1:65530"]
user_rules:
  - "||blocked.quench.test^"
schema_version: 34
`
		if err := os.WriteFile(filepath.Join(dir, "AdGuardHome.yaml"), []byte(cfg), 0o666); err != nil {
			t.Fatalf("%v", err)
		}
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", dir + ":/opt/adguardhome/conf", "--tmpfs", "/opt/adguardhome/work:rw,uid=1001,gid=1001",
			"-p", "127.0.0.1:13030:3000", "-p", "127.0.0.1:15353:5353/udp"})
		r := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "udp", "127.0.0.1:15353")
		}}
		var addrs []string
		t.waitFor(60, time.Second, name, "AdGuard Home never answered DNS", func() bool {
			if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
				t.Fatalf("AdGuard Home exited\n%s", t.Logs(name))
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			a, err := r.LookupHost(ctx, "blocked.quench.test")
			addrs = a
			return err == nil
		})
		if strings.Join(addrs, ",") != "0.0.0.0" && strings.Join(addrs, ",") != "0.0.0.0,::" && strings.Join(addrs, ",") != "::,0.0.0.0" {
			t.Fatalf("the blocked name resolved to %v, expected the 0.0.0.0 block answer", addrs)
		}
		login := func(p string) (int, *http.Client) {
			jar, _ := cookiejar.New(nil)
			cl := &http.Client{Jar: jar, Timeout: 10 * time.Second}
			resp, err := cl.Post("http://127.0.0.1:13030/control/login", "application/json", strings.NewReader(`{"name":"admin","password":"`+p+`"}`))
			if err != nil {
				return 0, cl
			}
			resp.Body.Close()
			return resp.StatusCode, cl
		}
		c, cl := login(pw)
		if c != 200 {
			t.Fatalf("admin login answered %d\n%s", c, t.Logs(name))
		}
		if resp, err := cl.Get("http://127.0.0.1:13030/control/status"); err != nil || resp.StatusCode != 200 {
			t.Fatalf("/control/status after login: %v %v", err, resp)
		} else {
			resp.Body.Close()
		}
		if c, _ := login("wrong-password-1"); c != 403 && c != 401 {
			t.Fatalf("a wrong password answered %d", c)
		}
		t.Logf("smoke test passed (adguardhome %s: read-only boot, seeded config, blocked name answers 0.0.0.0, admin login and status, wrong password refused; user %s)", t.Want, u)
	})
}
