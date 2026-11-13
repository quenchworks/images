package smoke

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// resp sends one RESP command and returns the reply: a simple string, an error ("-ERR ..."),
// an integer, or a bulk string. Arrays are not decoded.
func resp(addr string, args ...string) (string, error) {
	c, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return "", err
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(10 * time.Second))
	var b strings.Builder
	fmt.Fprintf(&b, "*%d\r\n", len(args))
	for _, a := range args {
		fmt.Fprintf(&b, "$%d\r\n%s\r\n", len(a), a)
	}
	if _, err := c.Write([]byte(b.String())); err != nil {
		return "", err
	}
	r := bufio.NewReader(c)
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimRight(line, "\r\n")
	if !strings.HasPrefix(line, "$") {
		return line[1:], nil
	}
	n, _ := strconv.Atoi(line[1:])
	if n < 0 {
		return "", nil
	}
	buf := make([]byte, n+2)
	_, err = io.ReadFull(r, buf)
	return string(buf[:n]), err
}

func init() {
	register("clickhouse", func(t *T) {
		const u, user, pass = "http://127.0.0.1:18123/", "smokeuser", "smokepass"
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/etc/clickhouse-server:rw,mode=1777", "--tmpfs", "/var/lib/clickhouse:rw,mode=1777,exec",
			"--tmpfs", "/var/log/clickhouse-server:rw,mode=1777", "-p", "127.0.0.1:18123:8123",
			"-e", "CLICKHOUSE_USER=" + user, "-e", "CLICKHOUSE_PASSWORD=" + pass, "-e", "CLICKHOUSE_DB=smoke"})
		if id := strings.TrimSpace(t.Docker("exec", name, "id", "-u")); id != "1001" {
			t.Fatalf("runtime uid %s, expected 1001", id)
		}
		t.waitFor(60, time.Second, name, "clickhouse did not answer /ping", func() bool {
			_, b := Get(u + "ping")
			return strings.Contains(b, "Ok.")
		})
		q := func(pw, sql string) (int, string) {
			return DoH("POST", u, sql, map[string]string{"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pw))})
		}
		must := func(sql string) string {
			c, b := q(pass, sql)
			if c != 200 {
				t.Fatalf("%s: HTTP %d %s", sql, c, b)
			}
			return strings.TrimSpace(b)
		}
		t.Logf("SELECT version() -> %s", must("SELECT version()"))
		must("CREATE TABLE IF NOT EXISTS smoke.t (id UInt32, name String) ENGINE = MergeTree ORDER BY id")
		must("INSERT INTO smoke.t VALUES (1, 'alpha'), (2, 'beta')")
		if n := must("SELECT count() FROM smoke.t"); n != "2" {
			t.Fatalf("expected 2 rows, got %s", n)
		}
		if v := must("SELECT name FROM smoke.t WHERE id = 2"); v != "beta" {
			t.Fatalf("expected 'beta', got %q", v)
		}
		if c, _ := q("wrongpass", "SELECT 1"); c == 200 {
			t.Fatalf("query succeeded with wrong password")
		}
		t.Logf("  wrong password correctly rejected")
		t.Logf("  client -> %s", strings.TrimSpace(t.Docker("exec", name, "clickhouse", "client", "--user", user, "--password", pass, "--query", "SELECT version()")))
		t.Logf("PASS: clickhouse smoke test green")
	})
	register("dragonfly", func(t *T) {
		name := t.Name("")
		ro := []string{"--read-only", "--tmpfs", "/data:rw,mode=1777", "--tmpfs", "/tmp:rw,mode=1777"}
		t.Start(name, append(ro, "-p", "127.0.0.1:16379:6379"))
		if id := strings.TrimSpace(t.Docker("exec", name, "id", "-u")); id != "1001" {
			t.Fatalf("runtime uid %s, expected 1001", id)
		}
		const addr = "127.0.0.1:16379"
		t.waitFor(60, time.Second, name, "Dragonfly did not answer PING", func() bool {
			r, _ := resp(addr, "PING")
			return r == "PONG"
		})
		// The operator probes with /bin/sh /usr/local/bin/healthcheck.sh.
		hc := func(n, port string) error {
			_, err := docker("exec", "-e", "HEALTHCHECK_PORT="+port, n, "/bin/sh", "/usr/local/bin/healthcheck.sh")
			return err
		}
		if err := hc(name, "6379"); err != nil {
			t.Fatalf("healthcheck.sh did not pass")
		}
		if hc(name, "6390") == nil {
			t.Fatalf("healthcheck.sh passed on a closed port")
		}
		// Container args must reach dragonfly: the operator passes --admin_port.
		args := t.Name("args")
		t.Start(args, ro, "--admin_port=9999", "--admin_nopass")
		t.waitFor(30, time.Second, args, "--admin_port=9999 was not applied", func() bool { return hc(args, "9999") == nil })
		docker("rm", "-f", args)
		t.Logf("  --admin_port=9999 answers PING")
		if r, err := resp(addr, "SET", "gate", "quench"); r != "OK" {
			t.Fatalf("SET: %q %v", r, err)
		}
		if v, _ := resp(addr, "GET", "gate"); v != "quench" {
			t.Fatalf("expected 'quench', got %q", v)
		}
		info, _ := resp(addr, "INFO", "server")
		t.ContainsFold(info, "dragonfly", "INFO server")
		t.Logf("PASS: dragonfly smoke test green (nonroot uid 1001, RESP on 6379)")
	})
	register("kubo", func(t *T) {
		net_ := t.Name("net")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		// local-discovery drops the server profile's private-range dial filters.
		nodes := map[string][2]string{"a": {"15001", "18080"}, "b": {"15002", "18081"}}
		names := map[string]string{}
		for n, p := range nodes {
			names[n] = t.Name(n)
			t.Start(names[n], []string{"--network", net_, "-e", "IPFS_PROFILE=local-discovery", "-p", "127.0.0.1:" + p[0] + ":5001", "-p", "127.0.0.1:" + p[1] + ":8080"})
		}
		api := func(n, path string) (int, string) {
			return Post("http://127.0.0.1:"+nodes[n][0]+"/api/v0/"+path, "", "")
		}
		field := func(body, k string) string {
			var m map[string]any
			json.Unmarshal([]byte(body), &m)
			s, _ := m[k].(string)
			return s
		}
		up := func() {
			for _, n := range []string{"a", "b"} {
				t.waitFor(60, time.Second, names[n], "RPC API never answered on "+n, func() bool { c, _ := api(n, "id"); return c == 200 })
			}
		}
		up()
		// mDNS would connect over TCP and satisfy the QUIC dial below; config is read at boot,
		// so turn it off and restart. The restart also proves the repo is reused (same PeerID).
		_, id := api("a", "id")
		peerA := field(id, "ID")
		for _, n := range []string{"a", "b"} {
			api(n, "config?arg=Discovery.MDNS.Enabled&arg=false&bool=true")
		}
		t.Docker("restart", names["a"], names["b"])
		up()
		if _, id := api("a", "id"); field(id, "ID") != peerA {
			t.Fatalf("restart re-initialized the repo")
		}
		_, vb := api("a", "version")
		ver := field(vb, "Version")
		t.Logf("reported version: %s", ver)
		if ver == "" || strings.Contains(ver, "dev") {
			t.Fatalf("version not stamped: %q", ver)
		}
		if t.Want != "" && ver != t.Want {
			t.Fatalf("expected %s, got %s", t.Want, ver)
		}
		payload := "quenchworks-kubo-" + hexRand(4)
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		fw, _ := mw.CreateFormFile("file", "smoke.txt")
		fw.Write([]byte(payload + "\n"))
		mw.Close()
		_, ab := Post("http://127.0.0.1:15001/api/v0/add?cid-version=1&quieter=true", mw.FormDataContentType(), body.String())
		cid := field(ab, "Hash")
		if cid == "" {
			t.Fatalf("add returned no CID: %s", ab)
		}
		if _, g := Get("http://127.0.0.1:18080/ipfs/" + cid); strings.TrimSpace(g) != payload {
			t.Fatalf("gateway A returned %q for %s", g, cid)
		}
		t.Logf("  node A: %s served by its own gateway", cid)
		ip := t.Docker("inspect", "-f", `{{(index .NetworkSettings.Networks "`+net_+`").IPAddress}}`, names["a"])
		addr := "/ip4/" + ip + "/udp/4001/quic-v1/p2p/" + peerA
		if c, b := api("b", "swarm/connect?arg="+addr); c != 200 {
			t.Fatalf("node B could not dial A over QUIC (%s): %s", addr, b)
		}
		var peers struct{ Peers []struct{ Addr, Peer string } }
		_, pb := api("b", "swarm/peers")
		json.Unmarshal([]byte(pb), &peers)
		quic := false
		for _, p := range peers.Peers {
			quic = quic || (p.Peer == peerA && strings.HasSuffix(p.Addr, "/udp/4001/quic-v1"))
		}
		if !quic {
			t.Fatalf("B is connected to A, but not over QUIC: %s", pb)
		}
		c := &http.Client{Timeout: 60 * time.Second}
		r, err := c.Post("http://127.0.0.1:15002/api/v0/cat?arg="+cid, "", nil)
		if err != nil {
			t.Fatalf("cat on B: %v", err)
		}
		var got bytes.Buffer
		got.ReadFrom(r.Body)
		r.Body.Close()
		if strings.TrimSpace(got.String()) != payload {
			t.Fatalf("node B fetched %q for %s", got.String(), cid)
		}
		t.Logf("  node B: dialed A over QUIC and fetched %s by bitswap", cid)
		t.Logf("smoke test passed (version %s, nonroot user: %s, QUIC + bitswap between two nodes, peer kept across restart)", ver, t.User("1001"))
	})
}
