package smoke

import (
	"encoding/json"
	"strings"
	"time"
)

func init() {
	// Dev mode (auto-unsealed) does a kv roundtrip with the in-image bao CLI and reports the
	// stamped version; the default raft server comes up sealed, then a real operator init and
	// shamir unseal (the seal wrapper and raft path dev mode hides) and a kv roundtrip.
	register("openbao", func(t *T) {
		u := t.User("1001")
		ro := []string{"--read-only", "--tmpfs", "/openbao/data:rw,mode=1777", "--tmpfs", "/openbao/config:rw,mode=1777", "--tmpfs", "/tmp:rw,mode=1777"}
		bao := func(name, token string, args ...string) (string, error) {
			a := []string{"exec", "-e", "BAO_ADDR=http://127.0.0.1:8200"}
			if token != "" {
				a = append(a, "-e", "BAO_TOKEN="+token)
			}
			return docker(append(append(a, name, "bao"), args...)...)
		}
		health := func(port string) int { c, _ := Get("http://127.0.0.1:" + port + "/v1/sys/health"); return c }

		dev := t.Name("dev")
		t.Start(dev, append(append([]string{}, ro...), "-p", "127.0.0.1:18200:8200", "-e", "BAO_DEV=1", "-e", "BAO_DEV_ROOT_TOKEN=root"))
		t.waitFor(30, time.Second, dev, "dev health never answered 200", func() bool { return health("18200") == 200 })
		if out, err := bao(dev, "root", "kv", "put", "secret/quench", "value=hello"); err != nil {
			t.Fatalf("dev kv put: %s", out)
		}
		if got, _ := bao(dev, "root", "kv", "get", "-field=value", "secret/quench"); got != "hello" {
			t.Fatalf("dev kv roundtrip: %q", got)
		}
		v, _ := docker("exec", dev, "bao", "version")
		// the stamp once missed (a moved package made -X a no-op) and the binary said v2.0.0-HEAD
		if t.Want != "" {
			t.Contains(v, "OpenBao v"+t.Want, "bao version")
		}

		srv := t.Name("srv")
		t.Start(srv, append(append([]string{}, ro...), "-p", "127.0.0.1:18300:8200"))
		t.waitFor(30, time.Second, srv, "raft node never listened (501 uninitialized or 503 sealed)", func() bool {
			c := health("18300")
			return c == 501 || c == 503
		})
		out, err := bao(srv, "", "operator", "init", "-key-shares=1", "-key-threshold=1", "-format=json")
		var init struct {
			Keys  []string `json:"unseal_keys_b64"`
			Token string   `json:"root_token"`
		}
		if err != nil || json.Unmarshal([]byte(out), &init) != nil || len(init.Keys) == 0 || init.Token == "" {
			t.Fatalf("operator init did not return a key and token\n%s", out)
		}
		if out, err := bao(srv, "", "operator", "unseal", init.Keys[0]); err != nil {
			t.Fatalf("unseal: %s", out)
		}
		t.waitFor(30, time.Second, srv, "raft node never unsealed", func() bool { return health("18300") == 200 })
		bao(srv, init.Token, "secrets", "enable", "-path=secret", "kv-v2")
		bao(srv, init.Token, "kv", "put", "secret/quench", "value=raft")
		if got, _ := bao(srv, init.Token, "kv", "get", "-field=value", "secret/quench"); got != "raft" {
			t.Fatalf("raft kv roundtrip: %q", got)
		}
		t.Logf("smoke test passed (openbao %s: dev kv roundtrip, raft init + unseal + kv; user %s)", t.Want, u)
	})

	// Production mode with a master key on a read-only rootfs: /health, the JSON status at / (the
	// dashboard is hidden in production), the mini-dashboard embedded in the binary and served in
	// development mode, and /keys with, without and with a wrong key.
	register("meilisearch", func(t *T) {
		const key = "testkey-smoke-0123456789abcdef"
		u := t.User("1001")
		ro := []string{"--read-only", "--tmpfs", "/meili_data:rw,mode=1777", "--tmpfs", "/tmp:rw,mode=1777"}
		up := func(name, base string) {
			t.waitFor(60, time.Second, name, "meilisearch /health did not become healthy", func() bool {
				if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
					t.Fatalf("container exited\n%s", t.Logs(name))
				}
				c, _ := Get(base + "/health")
				return c == 200
			})
		}
		name := t.Name("")
		const base = "http://127.0.0.1:17700"
		t.Start(name, append(append([]string{}, ro...), "-e", "MEILI_MASTER_KEY="+key, "-e", "MEILI_ENV=production", "-p", "127.0.0.1:17700:7700"))
		up(name, base)
		_, h := Get(base + "/health")
		t.Contains(h, `"available"`, "/health payload")
		c, root := Get(base + "/")
		if c != 200 || !strings.Contains(root, `"Meilisearch is running"`) {
			t.Fatalf("GET / in production: HTTP %d %q", c, root)
		}
		if _, err := docker("exec", name, "grep", "-aq", "Mini-dashboard", "/usr/bin/meilisearch"); err != nil {
			t.Fatalf("mini-dashboard assets are not embedded in the binary")
		}
		dev := t.Name("dev")
		t.Start(dev, append(append([]string{}, ro...), "-e", "MEILI_ENV=development", "-p", "127.0.0.1:17701:7700"))
		up(dev, "http://127.0.0.1:17701")
		_, dash := Get("http://127.0.0.1:17701/")
		t.Contains(dash, "Mini-dashboard", "development mode serves the mini-dashboard")
		keys := func(auth string) int {
			h := map[string]string{}
			if auth != "" {
				h["Authorization"] = "Bearer " + auth
			}
			c, _ := DoH("GET", base+"/keys", "", h)
			return c
		}
		if c := keys(key); c != 200 {
			t.Fatalf("GET /keys with the master key: %d", c)
		}
		for _, k := range []string{"", "wrong-key"} {
			if c := keys(k); c != 401 && c != 403 {
				t.Fatalf("GET /keys with key %q: %d, expected 401/403", k, c)
			}
		}
		v, _ := docker("exec", name, "/usr/bin/meilisearch", "--version")
		if t.Want != "" {
			t.Contains(v, t.Want, "meilisearch version")
		}
		t.Logf("smoke test passed (meilisearch %s: read-only rootfs, /health, dashboard embedded, auth with/without/wrong key; user %s)", t.Want, u)
	})
}
