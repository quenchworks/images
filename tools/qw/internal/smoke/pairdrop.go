package smoke

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// wsConn is a minimal WebSocket client (RFC 6455): text frames only, enough to drive a signaling server.
type wsConn struct {
	c net.Conn
	r *bufio.Reader
}

func wsDial(addr, path string) (*wsConn, error) {
	c, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	k := make([]byte, 16)
	_, _ = rand.Read(k)
	fmt.Fprintf(c, "GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n\r\n",
		path, addr, base64.StdEncoding.EncodeToString(k))
	r := bufio.NewReader(c)
	resp, err := http.ReadResponse(r, nil)
	if err != nil {
		c.Close()
		return nil, err
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		c.Close()
		return nil, fmt.Errorf("handshake: %s", resp.Status)
	}
	return &wsConn{c: c, r: r}, nil
}

// send writes one masked text frame (clients must mask).
func (w *wsConn) send(v any) error {
	p, _ := json.Marshal(v)
	h := []byte{0x81}
	switch {
	case len(p) < 126:
		h = append(h, 0x80|byte(len(p)))
	default:
		h = append(h, 0x80|126, byte(len(p)>>8), byte(len(p)))
	}
	m := make([]byte, 4)
	_, _ = rand.Read(m)
	h = append(h, m...)
	for i := range p {
		p[i] ^= m[i%4]
	}
	_, err := w.c.Write(append(h, p...))
	return err
}

// next reads frames until a text frame whose JSON "type" is want, or the deadline.
func (w *wsConn) next(want string, d time.Duration) (map[string]any, error) {
	_ = w.c.SetReadDeadline(time.Now().Add(d))
	for {
		h := make([]byte, 2)
		if _, err := io.ReadFull(w.r, h); err != nil {
			return nil, err
		}
		n := int64(h[1] & 0x7f)
		switch n {
		case 126:
			b := make([]byte, 2)
			if _, err := io.ReadFull(w.r, b); err != nil {
				return nil, err
			}
			n = int64(binary.BigEndian.Uint16(b))
		case 127:
			b := make([]byte, 8)
			if _, err := io.ReadFull(w.r, b); err != nil {
				return nil, err
			}
			n = int64(binary.BigEndian.Uint64(b))
		}
		p := make([]byte, n)
		if _, err := io.ReadFull(w.r, p); err != nil {
			return nil, err
		}
		if h[0]&0x0f != 1 {
			continue // ping, pong, binary
		}
		var m map[string]any
		if json.Unmarshal(p, &m) == nil && m["type"] == want {
			return m, nil
		}
	}
}

func init() {
	// On a read-only root: the page is served, and two WebSocket peers get their config and names, join the IP room
	// and see each other (peers / peer-joined), which is the signaling a browser transfer starts from.
	register("pairdrop", func(t *T) {
		const addr = "127.0.0.1:18129"
		u := t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-p", addr + ":3000"})
		t.waitFor(30, time.Second, name, "the page did not answer", func() bool {
			c, _ := Get("http://" + addr + "/")
			return c == http.StatusOK
		})
		_, page := Get("http://" + addr + "/")
		t.ContainsFold(page, "pairdrop", "page")
		peer := func() *wsConn {
			w, err := wsDial(addr, "/server/webrtc")
			if err != nil {
				t.Fatalf("websocket: %v\n%s", err, t.Logs(name))
			}
			if _, err := w.next("ws-config", 10*time.Second); err != nil {
				t.Fatalf("no ws-config: %v", err)
			}
			m, err := w.next("display-name", 10*time.Second)
			if err != nil || m["displayName"] == nil {
				t.Fatalf("no display-name: %v %v", err, m)
			}
			return w
		}
		a := peer()
		defer a.c.Close()
		if err := a.send(map[string]string{"type": "join-ip-room"}); err != nil {
			t.Fatalf("join a: %v", err)
		}
		b := peer()
		defer b.c.Close()
		if err := b.send(map[string]string{"type": "join-ip-room"}); err != nil {
			t.Fatalf("join b: %v", err)
		}
		m, err := b.next("peers", 10*time.Second)
		if err != nil {
			t.Fatalf("peer b got no peers list: %v", err)
		}
		if ps, _ := m["peers"].([]any); len(ps) != 1 {
			t.Fatalf("peer b sees %d peers, want 1: %v", len(ps), m)
		}
		if _, err := a.next("peer-joined", 10*time.Second); err != nil {
			t.Fatalf("peer a was not told b joined: %v", err)
		}
		t.Logf("smoke test passed (pairdrop %s: read-only root, page served, two WebSocket peers got config and names and saw each other in the IP room; user %s)", t.Want, u)
	})
}
