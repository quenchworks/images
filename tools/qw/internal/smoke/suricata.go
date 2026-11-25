package smoke

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
)

// suricataPcap returns a pcap file holding one Ethernet/IPv4/UDP packet carrying payload. Checksums stay zero: the
// smoke test runs Suricata with -k none.
func suricataPcap(payload string) []byte {
	var b bytes.Buffer
	le := binary.LittleEndian
	binary.Write(&b, le, []uint32{0xa1b2c3d4})
	binary.Write(&b, le, []uint16{2, 4})
	binary.Write(&b, le, []uint32{0, 0, 65535, 1}) // thiszone, sigfigs, snaplen, linktype Ethernet
	udp := append([]byte{0x30, 0x39, 0x00, 0x35, 0, 0, 0, 0}, payload...) // 12345 -> 53
	binary.BigEndian.PutUint16(udp[4:], uint16(len(udp)))
	ip := []byte{0x45, 0, 0, 0, 0, 1, 0, 0, 64, 17, 0, 0, 10, 0, 0, 1, 10, 0, 0, 2}
	binary.BigEndian.PutUint16(ip[2:], uint16(20+len(udp)))
	eth := []byte{0, 1, 2, 3, 4, 5, 0, 1, 2, 3, 4, 6, 0x08, 0x00}
	frame := append(append(eth, ip...), udp...)
	binary.Write(&b, le, []uint32{1_700_000_000, 0, uint32(len(frame)), uint32(len(frame))})
	b.Write(frame)
	return b.Bytes()
}

func init() {
	// As 1001 on a read-only rootfs: suricata reads a one-packet pcap offline (-r) with a local rule and writes the
	// alert to eve.json, the build info lists AF_PACKET, and the bundled suricata-update runs on the image's Python.
	register("suricata", func(t *T) {
		u := t.User("1001")
		t.Contains(t.Run("-V"), t.Want, "suricata -V")
		t.Contains(t.Run("--build-info"), "AF_PACKET", "build info")
		t.Contains(t.RunEntry("/usr/bin/suricata-update", "--version"), "suricata-update", "suricata-update --version")
		d, err := os.MkdirTemp("", "suricata-smoke")
		if err != nil {
			t.Fatalf("temp dir: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(d) })
		in, out := filepath.Join(d, "in"), filepath.Join(d, "out")
		os.MkdirAll(in, 0o755)
		os.MkdirAll(out, 0o777)
		os.Chmod(d, 0o755)
		os.Chmod(out, 0o777)
		os.WriteFile(filepath.Join(in, "smoke.pcap"), suricataPcap("QUENCH-EVIL-MARKER"), 0o644)
		os.WriteFile(filepath.Join(in, "local.rules"), []byte(
			`alert udp any any -> any any (msg:"quench smoke"; content:"QUENCH-EVIL-MARKER"; sid:1000001; rev:1;)`+"\n"), 0o644)
		res, err := t.RunFlags([]string{"--read-only", "--tmpfs", "/run/suricata:rw,uid=1001", "-v", in + ":/in:ro", "-v", out + ":/out"},
			"-r", "/in/smoke.pcap", "-S", "/in/local.rules", "-l", "/out", "-k", "none", "--runmode", "single")
		if err != nil {
			t.Fatalf("suricata -r: %v\n%s", err, res)
		}
		eve, err := os.ReadFile(filepath.Join(out, "eve.json"))
		if err != nil {
			t.Fatalf("no eve.json: %v\n%s", err, res)
		}
		if !strings.Contains(string(eve), `"event_type":"alert"`) || !strings.Contains(string(eve), `"signature_id":1000001`) {
			t.Fatalf("eve.json has no alert for sid 1000001:\n%s", eve)
		}
		t.Logf("smoke test passed (suricata %s: pcap read offline, local rule alerted in eve.json, AF_PACKET built in, suricata-update runs; user %s)", t.Want, u)
	})
}
