package smoke

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func init() {
	// Read-only as 1001 on the shipped config: a GET through the proxy reaches a host stub and carries the Via
	// header, and CONNECT to a port outside ConnectPort (443, 563) is refused.
	register("tinyproxy", func(t *T) {
		u := t.User("1001")
		stub := t.hostStub(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "via=%s", r.Header.Get("Via"))
		}))
		name := t.Name("")
		t.Start(name, append([]string{"--read-only", "-p", "127.0.0.1:18888:8888"}, hostStubFlag...))
		t.waitFor(30, time.Second, name, "never listened", func() bool {
			c, err := net.DialTimeout("tcp", "127.0.0.1:18888", time.Second)
			if err == nil {
				c.Close()
			}
			return err == nil
		})
		pu, _ := url.Parse("http://127.0.0.1:18888")
		c := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(pu)}, Timeout: 15 * time.Second}
		r, err := c.Get(stub + "/")
		if err != nil {
			t.Fatalf("GET through the proxy: %v\n%s", err, t.Logs(name))
		}
		b := make([]byte, 256)
		n, _ := r.Body.Read(b)
		r.Body.Close()
		if r.StatusCode != 200 || !strings.Contains(string(b[:n]), "tinyproxy") {
			t.Fatalf("proxied GET: %d %q\n%s", r.StatusCode, b[:n], t.Logs(name))
		}
		conn, err := net.DialTimeout("tcp", "127.0.0.1:18888", 5*time.Second)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		defer conn.Close()
		host := strings.TrimPrefix(stub, "http://")
		fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", host, host)
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		line, _ := bufio.NewReader(conn).ReadString('\n')
		if !strings.Contains(line, " 403 ") {
			t.Fatalf("CONNECT to a non-allowed port answered %q, want 403", strings.TrimSpace(line))
		}
		t.Contains(t.Logs(name), "tinyproxy", "startup log")
		t.Logf("smoke test passed (tinyproxy %s: read-only boot, proxied GET with Via, CONNECT outside ConnectPort refused; user %s)", t.Want, u)
	})
}
