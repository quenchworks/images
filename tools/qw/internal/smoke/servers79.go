package smoke

import (
	"crypto/x509"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

func init() {
	// The network validator runs; then the proxy-identity wrapper writes key.p8 and a CSR naming
	// the local identity and execs the proxy, whose admin server answers /live and /metrics with
	// identity, destination and policy unreachable by design (without a policy service the proxy
	// exits 64 "Invalid configuration", which the bash test accepted).
	register("linkerd-proxy", func(t *T) {
		const local = "test-proxy.default.serviceaccount.identity.linkerd.cluster.local"
		const base = "http://127.0.0.1:14191"
		t.Contains(t.RunEntryAny("/usr/lib/linkerd/linkerd2-network-validator", "--help"), "--connect-addr", "network validator --help")
		w := t.TempDir()
		id, tok := filepath.Join(w, "identity"), filepath.Join(w, "token")
		os.MkdirAll(id, 0o777)
		os.Chmod(id, 0o777)
		os.MkdirAll(tok, 0o755)
		os.WriteFile(filepath.Join(tok, "linkerd-identity-token"), []byte("test-token\n"), 0o644)
		caPEM, _, _ := rsaCAAndLeaf("identity.linkerd.cluster.local")
		name := t.Name("")
		env := map[string]string{
			"ADMIN_LISTEN_ADDR":            "0.0.0.0:4191",
			"IDENTITY_TRUST_ANCHORS":       string(caPEM),
			"IDENTITY_IDENTITY_LOCAL_NAME": local,
			"IDENTITY_LOCAL_NAME":          local,
			"IDENTITY_DIR":                 "/var/run/linkerd/identity/end-entity",
			"IDENTITY_TOKEN_FILE":          "/var/run/secrets/tokens/linkerd-identity-token",
			"IDENTITY_SVC_ADDR":            "127.0.0.1:8080",
			"IDENTITY_SVC_NAME":            "linkerd-identity.linkerd.serviceaccount.identity.linkerd.cluster.local",
			"DESTINATION_SVC_ADDR":         "127.0.0.1:8086",
			"DESTINATION_SVC_NAME":         "linkerd-destination.linkerd.serviceaccount.identity.linkerd.cluster.local",
			"POLICY_SVC_ADDR":              "127.0.0.1:8090",
			"POLICY_SVC_NAME":              "linkerd-destination.linkerd.serviceaccount.identity.linkerd.cluster.local",
			"POLICY_WORKLOAD":              `{"ns":"default","pod":"test-proxy"}`,
		}
		flags := []string{"-p", "127.0.0.1:14191:4191", "-v", id + ":/var/run/linkerd/identity/end-entity", "-v", tok + ":/var/run/secrets/tokens:ro"}
		for k, v := range env {
			flags = append(flags, "-e", "LINKERD2_PROXY_"+k+"="+v)
		}
		t.Start(name, flags)
		var csrDER []byte
		t.waitFor(20, time.Second, name, "linkerd2-proxy-identity did not write key.p8 and csr.der", func() bool {
			k, _ := os.ReadFile(filepath.Join(id, "key.p8"))
			csrDER, _ = os.ReadFile(filepath.Join(id, "csr.der"))
			return len(k) > 0 && len(csrDER) > 0
		})
		csr, err := x509.ParseCertificateRequest(csrDER)
		if err != nil || (csr.Subject.CommonName != local && !slices.Contains(csr.DNSNames, local)) {
			t.Fatalf("csr.der does not name %s (%v)", local, err)
		}
		t.waitFor(20, time.Second, name, "admin /live never answered 200", func() bool {
			if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
				t.Fatalf("proxy exited (exit %s)\n%s", t.Docker("inspect", "-f", "{{.State.ExitCode}}", name), t.Logs(name))
			}
			c, _ := Get(base + "/live")
			return c == 200
		})
		u := t.User("1001")
		_, m := Get(base + "/metrics")
		t.Match(m, `(?m)^# HELP `, "Prometheus /metrics")
		c, _ := Get(base + "/ready")
		t.Logf("smoke test passed (linkerd-proxy %s: identity key + CSR, /live 200, /metrics, /ready %d without control plane; user %s)", t.Want, c, u)
	})

	// The default command is a raft server on a read-only rootfs: 501 until operator init, shamir
	// unseal, kv-v2 roundtrip, the Ember UI built in (-tags ui) with an asset that serves; then dev
	// mode auto-unseals and does a kv roundtrip.
	register("vault", func(t *T) {
		u := t.User("1001")
		if cmd := t.Docker("inspect", t.Image, "--format", `{{join .Config.Cmd " "}}`); cmd != "server -config=/etc/vault/vault.hcl" {
			t.Fatalf("unexpected Cmd: %q", cmd)
		}
		health := func(port string) int { c, _ := Get("http://127.0.0.1:" + port + "/v1/sys/health"); return c }
		vault := func(name, token string, args ...string) string {
			a := []string{"exec"}
			if token != "" {
				a = append(a, "-e", "VAULT_TOKEN="+token)
			}
			return t.Docker(append(append(a, name, "vault"), args...)...)
		}
		srv := t.Name("srv")
		t.Start(srv, []string{"--read-only", "--tmpfs", "/vault/data:rw,mode=1777", "--tmpfs", "/tmp:rw,mode=1777", "-p", "127.0.0.1:18210:8200"})
		t.waitFor(40, time.Second, srv, "health never reported 501 (up, uninitialized)", func() bool { return health("18210") == 501 })
		var init struct {
			Keys  []string `json:"unseal_keys_b64"`
			Token string   `json:"root_token"`
		}
		out := vault(srv, "", "operator", "init", "-key-shares=1", "-key-threshold=1", "-format=json")
		if json.Unmarshal([]byte(out), &init) != nil || len(init.Keys) == 0 || init.Token == "" {
			t.Fatalf("operator init did not return a key and token\n%s", out)
		}
		vault(srv, "", "operator", "unseal", init.Keys[0])
		t.waitFor(40, time.Second, srv, "never unsealed", func() bool { return health("18210") == 200 })
		t.Match(vault(srv, "", "status"), `(?m)^Sealed +false$`, "vault status Sealed false")
		vault(srv, init.Token, "secrets", "enable", "-path=secret", "kv-v2")
		vault(srv, init.Token, "kv", "put", "secret/quench", "value=hello")
		if got := vault(srv, init.Token, "kv", "get", "-field=value", "secret/quench"); got != "hello" {
			t.Fatalf("kv roundtrip: %q", got)
		}
		_, ui := Get("http://127.0.0.1:18210/ui/")
		if strings.Contains(strings.ToLower(ui), "not available in this binary") {
			t.Fatalf("UI stub served: the binary was built without -tags ui")
		}
		asset := regexp.MustCompile(`/ui/assets/[A-Za-z0-9._-]+\.js`).FindString(ui)
		if asset == "" {
			t.Fatalf("no Ember bundle referenced in /ui/")
		}
		if c, _ := Get("http://127.0.0.1:18210" + asset); c != 200 {
			t.Fatalf("UI asset %s answered %d", asset, c)
		}
		v := vault(srv, "", "version")
		if t.Want != "" {
			t.Contains(v, t.Want, "vault version")
		}

		dev := t.Name("dev")
		t.Start(dev, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-p", "127.0.0.1:18310:8200", "-e", "VAULT_TOKEN=root", "--entrypoint", "/usr/bin/vault"},
			"server", "-dev", "-dev-listen-address=0.0.0.0:8200", "-dev-root-token-id=root", "-dev-no-store-token")
		t.waitFor(40, time.Second, dev, "dev node never healthy", func() bool { return health("18310") == 200 })
		vault(dev, "", "kv", "put", "secret/quench", "value=devhello")
		if got := vault(dev, "", "kv", "get", "-field=value", "secret/quench"); got != "devhello" {
			t.Fatalf("dev kv roundtrip: %q", got)
		}
		t.Logf("smoke test passed (%s: raft init + unseal + kv, UI asset serves, dev kv; user %s)", v, u)
	})
}
