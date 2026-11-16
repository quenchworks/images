package smoke

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/pem"
	"hash/crc32"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// selfSigned writes <host>.crt and <host>.key (PEM) for host into dir.
func selfSigned(dir, host string) error {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: host}, DNSNames: []string{host},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &k.PublicKey, k)
	if err != nil {
		return err
	}
	kb, _ := x509.MarshalECPrivateKey(k)
	if err := os.WriteFile(filepath.Join(dir, host+".crt"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, host+".key"), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb}), 0o644)
}

// stunBinding sends a Tailscale-style STUN binding request (SOFTWARE "tailnode" and a FINGERPRINT, which
// derper's parser requires) and reports whether a success response with the same transaction id came back.
func stunBinding(addr string) bool {
	c, err := net.Dial("udp", addr)
	if err != nil {
		return false
	}
	defer c.Close()
	const software = "tailnode"
	txid := make([]byte, 12)
	_, _ = rand.Read(txid)
	req := []byte{0x00, 0x01}                                           // binding request
	req = binary.BigEndian.AppendUint16(req, uint16(4+len(software)+8)) // attributes length
	req = append(req, 0x21, 0x12, 0xA4, 0x42)                           // magic cookie
	req = append(req, txid...)
	req = binary.BigEndian.AppendUint16(req, 0x8022) // SOFTWARE
	req = binary.BigEndian.AppendUint16(req, uint16(len(software)))
	req = append(req, software...)
	fp := crc32.ChecksumIEEE(req) ^ 0x5354554e
	req = binary.BigEndian.AppendUint16(req, 0x8028) // FINGERPRINT
	req = binary.BigEndian.AppendUint16(req, 4)
	req = binary.BigEndian.AppendUint32(req, fp)
	_ = c.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := c.Write(req); err != nil {
		return false
	}
	resp := make([]byte, 512)
	n, err := c.Read(resp)
	return err == nil && n >= 20 && resp[0] == 0x01 && resp[1] == 0x01 && bytes.Equal(resp[8:20], txid)
}

// sniGet fetches url over TLS with SNI set to host (derper refuses a handshake whose SNI is not its hostname).
func sniGet(url, host string) (int, string) {
	c := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{ServerName: host, InsecureSkipVerify: true}}}
	r, err := c.Get(url)
	if err != nil {
		return 0, err.Error()
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return r.StatusCode, string(b)
}

func init() {
	// derper on a read-only rootfs, nonroot on 443 (unprivileged_port_start=0): HTTPS with the manual
	// certificate serves the DERP home page and the probe endpoint, and STUN answers a binding request on
	// 3478/udp.
	register("derper", func(t *T) {
		const host = "derp.quench.test"
		u := t.User("1001")
		t.Contains(t.RunAny("-version"), t.Want, "derper -version")
		certs := t.TempDir()
		if err := selfSigned(certs, host); err != nil {
			t.Fatalf("certificate: %v", err)
		}
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/var/lib/derper:rw,uid=1001,gid=1001", "--sysctl", "net.ipv4.ip_unprivileged_port_start=0",
			"-v", certs + ":/certs:ro", "-p", "127.0.0.1:18443:443", "-p", "127.0.0.1:13478:3478/udp"},
			"-a", ":443", "-http-port", "-1", "-stun-port", "3478", "-certmode", "manual", "-certdir", "/certs", "-hostname", host, "-c", "/var/lib/derper/derper.json")
		var home string
		t.waitFor(60, time.Second, name, "derper never served HTTPS", func() bool {
			code, body := sniGet("https://127.0.0.1:18443/", host)
			home = body
			return code == http.StatusOK
		})
		t.Contains(home, "DERP", "the DERP home page")
		if code, _ := sniGet("https://127.0.0.1:18443/derp/probe", host); code != http.StatusOK {
			t.Fatalf("/derp/probe answered %d\n%s", code, t.Logs(name))
		}
		if !stunBinding("127.0.0.1:13478") {
			t.Fatalf("no STUN binding response on 3478/udp\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed (derper %s: nonroot TLS on 443 with a manual certificate, home page and probe endpoint, STUN binding response; user %s)", t.Want, u)
	})
}
