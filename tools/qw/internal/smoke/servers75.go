package smoke

import (
	"net"
	"strings"
	"time"

	"github.com/gocql/gocql"
)

func init() {
	// Single-node developer-mode boot on a read-only rootfs (tmpfs with exec: the libreloc loader
	// needs it): the REST API reports the release version, then a CQL keyspace, table, inserts and
	// selects round-trip on 9042.
	register("scylladb", func(t *T) {
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/etc/scylla:rw,mode=1777,exec", "--tmpfs", "/var/lib/scylla:rw,mode=1777,exec",
			"--tmpfs", "/tmp:rw,mode=1777,exec", "-p", "127.0.0.1:19042:9042", "-p", "127.0.0.1:11000:10000"})
		if id := t.Docker("exec", name, "id", "-u"); id != "1001" {
			t.Fatalf("runs as uid %q, expected 1001", id)
		}
		var ver string
		t.waitFor(120, time.Second, name, "REST API never came up", func() bool {
			c, b := Get("http://127.0.0.1:11000/storage_service/scylla_release_version")
			ver = strings.Trim(b, "\" \n")
			return c == 200 && ver != ""
		})
		if t.Want != "" && !strings.Contains(ver, t.Want) {
			t.Fatalf("release version %q, expected %s", ver, t.Want)
		}
		// the node advertises its container address; every connection goes to the published port
		cl := gocql.NewCluster("127.0.0.1:19042")
		cl.ProtoVersion = 4
		cl.Timeout = 30 * time.Second
		cl.ConnectTimeout = 10 * time.Second
		cl.DisableInitialHostLookup = true
		cl.AddressTranslator = gocql.AddressTranslatorFunc(func(net.IP, int) (net.IP, int) { return net.ParseIP("127.0.0.1"), 19042 })
		var s *gocql.Session
		t.waitFor(120, 2*time.Second, name, "CQL never accepted a session", func() bool {
			var err error
			s, err = cl.CreateSession()
			return err == nil
		})
		defer s.Close()
		// NetworkTopologyStrategy: Scylla 2026.x creates tablet keyspaces and rejects SimpleStrategy
		for _, q := range []string{
			"CREATE KEYSPACE IF NOT EXISTS smoke WITH replication = {'class': 'NetworkTopologyStrategy', 'replication_factor': 1}",
			"CREATE TABLE IF NOT EXISTS smoke.t (id int PRIMARY KEY, name text)",
		} {
			if err := s.Query(q).Exec(); err != nil {
				t.Fatalf("%s: %v", q, err)
			}
		}
		for id, n := range map[int]string{1: "alpha", 2: "beta"} {
			if err := s.Query("INSERT INTO smoke.t (id, name) VALUES (?, ?)", id, n).Exec(); err != nil {
				t.Fatalf("insert: %v", err)
			}
		}
		var got string
		if err := s.Query("SELECT name FROM smoke.t WHERE id = 2").Scan(&got); err != nil || got != "beta" {
			t.Fatalf("select id=2: %q %v", got, err)
		}
		var cnt int64
		if err := s.Query("SELECT count(*) FROM smoke.t").Scan(&cnt); err != nil || cnt != 2 {
			t.Fatalf("count: %d %v", cnt, err)
		}
		t.Logf("smoke test passed (scylladb %s: read-only rootfs, REST API, CQL keyspace/table/insert/select; uid 1001)", ver)
	})
}
