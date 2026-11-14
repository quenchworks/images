package smoke

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func init() {
	// Vaultwarden runs on a read-only root with /data on a volume. The test does what a Bitwarden
	// client does: derive the master password hash (PBKDF2), register, log in with the password
	// grant, store a cipher and read it back from /api/sync, also after a restart. A wrong password
	// must be refused and the web vault must be served.
	register("vaultwarden", func(t *T) {
		u := t.User("1001")
		v := t.RunEntryAny("/usr/bin/vaultwarden", "--version")
		t.Logf("%s", v)
		t.WantVersion(v)

		vol, name := t.Name("data"), t.Name("")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		var base string
		start := func() {
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:uid=1001,gid=1001", "-v", vol + ":/data", "-p", "127.0.0.1::8080"})
			base = "http://127.0.0.1:" + t.hostPort(name, "8080")
			t.WaitHTTP(base+"/alive", 60, name)
		}
		start()
		_, page := Get(base + "/")
		t.ContainsFold(page, "<html", "web vault")
		t.Logf("  /alive ok, web vault served")

		const email = "smoke@quench.test"
		const enc = "2.c21va2VpdjEyMzQ1Njc4OQ==|c21va2VjdA==|c21va2VtYWNtYWNtYWNtYWNtYWNtYWNtYWNtYWM="
		// Bitwarden's master password hash: PBKDF2-SHA256(PBKDF2-SHA256(password, email, 600000), password, 1)
		mhash := func(pw string) string {
			k, _ := pbkdf2.Key(sha256.New, pw, []byte(email), 600000, 32)
			h, _ := pbkdf2.Key(sha256.New, string(k), []byte(pw), 1, 32)
			return base64.StdEncoding.EncodeToString(h)
		}
		js := map[string]string{"Content-Type": "application/json", "Accept": "application/json"}
		post := func(path string, v any, hdr map[string]string) (int, string) {
			b, _ := json.Marshal(v)
			return DoH("POST", base+path, string(b), hdr)
		}
		login := func(pw string) (int, string) {
			dev := make([]byte, 16)
			rand.Read(dev)
			f := url.Values{"grant_type": {"password"}, "username": {email}, "password": {mhash(pw)}, "scope": {"api offline_access"},
				"client_id": {"web"}, "deviceType": {"9"}, "deviceIdentifier": {fmt.Sprintf("%x-%x-%x-%x-%x", dev[0:4], dev[4:6], dev[6:8], dev[8:10], dev[10:])}, "deviceName": {"smoke"}}
			return DoH("POST", base+"/identity/connect/token", f.Encode(), map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Accept": "application/json"})
		}
		const pw = "smoke-master-pass-2026"
		token := func() string {
			code, body := login(pw)
			var r struct {
				AccessToken string `json:"access_token"`
			}
			if json.Unmarshal([]byte(body), &r); code != 200 || r.AccessToken == "" {
				t.Fatalf("login failed %d %.300s", code, body)
			}
			return r.AccessToken
		}
		sync := func() {
			code, body := DoH("GET", base+"/api/sync", "", map[string]string{"Authorization": "Bearer " + token(), "Accept": "application/json"})
			var r struct {
				Ciphers []json.RawMessage `json:"ciphers"`
			}
			if json.Unmarshal([]byte(body), &r); code != 200 || len(r.Ciphers) != 1 {
				t.Fatalf("cipher not in /api/sync (%d, %d ciphers): %.300s", code, len(r.Ciphers), body)
			}
		}

		// two steps since 1.35: with mail off the verification call returns the token itself
		code, vtok := post("/identity/accounts/register/send-verification-email", map[string]string{"email": email, "name": "Smoke"}, js)
		if code != 200 {
			t.Fatalf("verification %d %.300s", code, vtok)
		}
		var vt any
		json.Unmarshal([]byte(vtok), &vt)
		code, body := post("/identity/accounts/register/finish", map[string]any{"email": email, "name": "Smoke", "masterPasswordHash": mhash(pw),
			"masterPasswordHint": nil, "key": enc, "kdf": 0, "kdfIterations": 600000,
			"keys": map[string]string{"publicKey": "c21va2UtcHVibGljLWtleQ==", "encryptedPrivateKey": enc}, "emailVerificationToken": vt}, js)
		if code != 200 && code != 204 {
			t.Fatalf("register failed %d %.300s\n--- logs:\n%s", code, body, t.Logs(name))
		}
		if bad, _ := login("wrong-master-pass"); bad != 400 && bad != 401 {
			t.Fatalf("a wrong password got HTTP %d", bad)
		}
		tok := token()
		claims, _ := base64.RawURLEncoding.DecodeString(strings.Split(tok, ".")[1])
		var c struct{ Sub string }
		json.Unmarshal(claims, &c)
		code, body = post("/api/ciphers", map[string]any{"type": 2, "name": enc, "notes": nil, "secureNote": map[string]int{"type": 0},
			"favorite": false, "encryptedFor": c.Sub}, map[string]string{"Content-Type": "application/json", "Accept": "application/json", "Authorization": "Bearer " + tok})
		if code != 200 {
			t.Fatalf("storing a cipher failed %d %.300s\n--- logs:\n%s", code, body, t.Logs(name))
		}
		sync()
		t.Logf("  registered, wrong password refused, cipher stored and synced")
		docker("rm", "-f", name)
		start()
		sync()
		t.Logf("  cipher kept across a restart")
		if l := strings.ToLower(t.Logs(name)); strings.Contains(l, "read-only file system") || strings.Contains(l, "panicked") {
			t.Fatalf("crashed or wrote to the root:\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed (vaultwarden %s: Bitwarden register/login, cipher stored and synced across a restart on /data, read-only root; user %s)", t.Want, u)
	})
}

func init() {
	// Boots the proxy on a read-only root with a minimal config (SAML2 backend exposing its
	// metadata, ping frontend), hits both endpoints, then signs the proxy metadata with the
	// satosa-saml-metadata CLI (pysaml2 -> xmlsec1) and verifies the signature with xmlsec1.
	register("satosa", func(t *T) {
		u := t.User("1001")
		ver := strings.TrimSpace(t.RunEntry("/opt/satosa/venv/bin/python", "-c", "import importlib.metadata as m; print(m.version('satosa'))"))
		if t.Want != "" && ver != t.Want {
			t.Fatalf("version mismatch: %s", ver)
		}
		t.Logf("%s", t.RunEntry("/usr/bin/xmlsec1", "--version"))

		// BASE is in the config and the metadata, so the host port is fixed
		const b = "http://127.0.0.1:18080"
		w := t.TempDir()
		k, _ := rsa.GenerateKey(rand.Reader, 2048)
		tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "satosa-smoke"},
			NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(48 * time.Hour)}
		der, _ := x509.CreateCertificate(rand.Reader, tpl, tpl, &k.PublicKey, k)
		kd, _ := x509.MarshalPKCS8PrivateKey(k)
		write := func(name string, b []byte) {
			if err := os.WriteFile(filepath.Join(w, name), b, 0o644); err != nil {
				t.Fatalf("write %s: %v", name, err)
			}
		}
		write("tls.crt", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
		write("tls.key", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kd}))
		write("proxy_conf.yaml", []byte(fmt.Sprintf(`BASE: %[1]s
COOKIE_STATE_NAME: SATOSA_STATE
STATE_ENCRYPTION_KEY: smoke-state-key-%[2]d-0123456789
INTERNAL_ATTRIBUTES:
  attributes:
    mail:
      saml: [mail]
BACKEND_MODULES:
  - module: satosa.backends.saml2.SAMLBackend
    name: Saml2
    config:
      entityid_endpoint: true
      sp_config:
        entityid: %[1]s/Saml2/metadata.xml
        key_file: /etc/satosa/tls.key
        cert_file: /etc/satosa/tls.crt
        metadata:
          local: []
        service:
          sp:
            endpoints:
              assertion_consumer_service:
                - [%[1]s/Saml2/acs/post, "urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST"]
FRONTEND_MODULES:
  - module: satosa.frontends.ping.PingFrontend
    name: ping
    config: {}
`, b, time.Now().UnixNano())))
		out := filepath.Join(w, "out")
		os.Mkdir(out, 0o777)
		os.Chmod(out, 0o777)
		mounts := []string{"--read-only", "--tmpfs", "/tmp", "-v", w + "/proxy_conf.yaml:/etc/satosa/proxy_conf.yaml:ro",
			"-v", w + "/tls.key:/etc/satosa/tls.key:ro", "-v", w + "/tls.crt:/etc/satosa/tls.crt:ro"}

		name := t.Name("")
		t.Start(name, append(mounts, "-p", "127.0.0.1:18080:8080"))
		t.WaitHTTP(b+"/ping", 60, name)
		md := t.WaitHTTP(b+"/Saml2/metadata.xml", 5, name)
		t.Contains(md, `entityID="`+b+`/Saml2/metadata.xml"`, "SP metadata")
		t.Contains(md, "X509Certificate", "certificate in the metadata")

		// signed metadata through pysaml2 + xmlsec1, then the verify command pysaml2 runs. One
		// backend, so backend.xml is a bare EntityDescriptor, not an EntitiesDescriptor.
		t.Docker(append(append([]string{"run", "--rm"}, mounts...), "-v", out+":/out", "--entrypoint", "/opt/satosa/venv/bin/satosa-saml-metadata",
			t.Image, "/etc/satosa/proxy_conf.yaml", "/etc/satosa/tls.key", "/etc/satosa/tls.crt", "--dir", "/out")...)
		signed, _ := os.ReadFile(filepath.Join(out, "backend.xml"))
		t.Contains(string(signed), "SignatureValue", "signed backend.xml")
		t.Docker(append(append([]string{"run", "--rm"}, mounts...), "-v", out+":/out:ro", "--entrypoint", "/usr/bin/xmlsec1", t.Image,
			"--verify", "--enabled-reference-uris", "empty,same-doc", "--enabled-key-data", "raw-x509-cert", "--lax-key-search",
			"--pubkey-cert-pem", "/etc/satosa/tls.crt", "--id-attr:ID", "urn:oasis:names:tc:SAML:2.0:metadata:EntityDescriptor", "/out/backend.xml")...)
		if m := regexp.MustCompile(`Traceback|CRITICAL`).FindString(t.Logs(name)); m != "" {
			t.Fatalf("satosa logged errors:\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed (satosa %s, uid %s, /ping + SP metadata served, metadata signed and verified by xmlsec1)", ver, u)
	})
}

func init() {
	// The image carries upstream's four control-plane binaries at /usr/local/bin. Each must report
	// the release; against a dead API server the controller manager must start and dial it.
	register("tidb-operator", func(t *T) {
		u := t.User("1001")
		t.LabelVersion()
		for _, b := range []string{"tidb-scheduler", "tidb-admission-webhook"} {
			v := t.RunEntryAny("/usr/local/bin/"+b, "-V")
			t.WantVersionV(v)
		}
		if n := len(t.Files("usr/local/bin/tidb-")); n != 4 {
			t.Fatalf("expected 4 tidb-* binaries, found %d", n)
		}
		kc := t.DeadKubeconfig()
		out, _ := t.RunAnyFor(60*time.Second, []string{"--read-only", "--tmpfs", "/tmp", "-v", kc + ":/w/kubeconfig:ro",
			"-e", "KUBECONFIG=/w/kubeconfig", "-e", "NAMESPACE=tidb-admin"})
		t.Contains(out, "127.0.0.1:1", "tidb-controller-manager reached the API server step")
		t.Logf("smoke test passed (tidb-operator %s: four binaries report the release, controller manager dials the API server; nonroot user %s)", t.Want, u)
	})
}

func init() {
	// Upstream's bash entrypoint dispatches operator|webhook to the shaded jars; against a dead
	// API server the operator JVM must start from the jars and fail at the dial.
	register("flink-kubernetes-operator", func(t *T) {
		u := t.User("1001")
		t.LabelVersion()
		t.Contains(t.RunAny("help"), "(operator|webhook)", "entrypoint usage")
		for _, j := range []string{"flink-kubernetes-operator/flink-kubernetes-operator-", "flink-kubernetes-operator/flink-kubernetes-webhook-"} {
			if len(t.Files(j)) == 0 {
				t.Fatalf("%s*.jar missing", j)
			}
		}
		kc := t.DeadKubeconfig()
		out, _ := t.RunAnyFor(90*time.Second, []string{"--read-only", "--tmpfs", "/tmp", "-v", kc + ":/w/kubeconfig:ro", "-e", "KUBECONFIG=/w/kubeconfig"}, "operator")
		t.Contains(out, "Starting Operator", "entrypoint")
		t.Match(out, `127\.0\.0\.1:1|java\.net\.ConnectException`, "operator reached the API server step")
		if regexp.MustCompile(`ClassNotFoundException|NoClassDefFoundError|UnsupportedClassVersionError`).MatchString(out) {
			t.Fatalf("classpath broken:\n%s", out)
		}
		t.Logf("smoke test passed (flink-kubernetes-operator %s: entrypoint dispatches, operator JVM starts and dials the API server; nonroot user %s)", t.Want, u)
	})
}
