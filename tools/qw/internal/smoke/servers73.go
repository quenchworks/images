package smoke

import (
	"regexp"
	"strings"
	"time"
)

func init() {
	// The image's default entrypoint boots the all-in-one server on an embedded sqlite store that
	// creates its own schema: the services start, the frontend gRPC (7233) and tally metrics (8000)
	// listen, and the Postgres schema files the chart uses are shipped.
	register("temporal", func(t *T) {
		u := t.User("1001")
		v := t.RunEntryAny("/usr/bin/temporal-server", "--version")
		if t.Want != "" {
			t.Contains(v, t.Want, "temporal-server version")
		}
		t.RunEntryAny("/usr/bin/temporal-sql-tool", "--version")
		if out, err := docker("run", "--rm", "--entrypoint", "/bin/sh", t.Image, "-c",
			"test -f /etc/temporal/schema/postgresql/v12/temporal/schema.sql && test -f /etc/temporal/schema/postgresql/v12/visibility/schema.sql && test -d /etc/temporal/schema/postgresql/v12/temporal/versioned"); err != nil {
			t.Fatalf("postgres schema files missing\n%s", out)
		}
		name := t.Name("")
		t.Start(name, []string{"--tmpfs", "/tmp:rw", "--tmpfs", "/data:rw,uid=1001,gid=1001", "-w", "/data"})
		started := regexp.MustCompile(`(?i)frontend started|Service resources started|Starting to serve`)
		t.waitFor(90, time.Second, name, "frontend did not report started", func() bool {
			if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
				t.Fatalf("container exited early\n%s", t.Logs(name))
			}
			return started.MatchString(t.Logs(name))
		})
		// no curl in the image: read the listening sockets (7233 = 0x1C41, 8000 = 0x1F40)
		listening := func(hexPort string) bool {
			out, _ := docker("exec", name, "/bin/sh", "-c", "cat /proc/net/tcp /proc/net/tcp6 2>/dev/null")
			return regexp.MustCompile(`(?im)^\s*\d+:\s+\S+:` + hexPort + `\s`).MatchString(out)
		}
		t.waitFor(30, time.Second, name, "frontend gRPC :7233 not listening", func() bool { return listening("1C41") })
		if !listening("1F40") {
			t.Fatalf("metrics :8000 not listening\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed (temporal %s: all-in-one sqlite boot, :7233 and :8000 listening, schema files shipped; user %s)", t.Want, u)
	})

	// MongoDB on a read-only rootfs with tmpfs /data: the entrypoint's auth bootstrap finishes, the
	// root user pings, a document round-trips, and an unauthenticated admin command is refused.
	register("mongodb", func(t *T) {
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/data:rw,mode=1777,exec", "-e", "MONGO_INITDB_ROOT_USERNAME=root",
			"-e", "MONGO_INITDB_ROOT_PASSWORD=quenchtest", "-e", "MONGO_INITDB_DATABASE=quench"})
		if id := t.Docker("exec", name, "id", "-u"); id != "1001" {
			t.Fatalf("runs as uid %q, expected 1001", id)
		}
		// mongosh writes under $HOME; the rootfs is read-only, so point it at /data/log
		m := func(uri, eval string) (string, error) {
			out, err := docker("exec", "-e", "HOME=/data/log", name, "mongosh", "--quiet", uri, "--eval", eval)
			return strings.TrimSpace(out), err
		}
		const root = "mongodb://root:quenchtest@localhost:27017/"
		// the entrypoint first runs a temporary no-auth mongod on the same port to create the user:
		// only a ping after its final start line proves the --auth server
		t.waitFor(90, time.Second, name, "mongod never accepted an authenticated connection", func() bool {
			if !strings.Contains(t.Logs(name), "starting mongod WITH --auth") {
				return false
			}
			out, _ := m(root+"admin?authSource=admin", "db.runCommand({ping:1}).ok")
			return out == "1"
		})
		q := root + "quench?authSource=admin"
		ver, _ := m(q, "db.version()")
		if t.Want != "" && !strings.HasPrefix(ver, t.Want) {
			t.Fatalf("db.version() %q, expected %s", ver, t.Want)
		}
		m(q, `db.gate.insertOne({k:"quench"})`)
		if got, _ := m(q, "db.gate.findOne().k"); got != "quench" {
			t.Fatalf("findOne().k = %q, expected quench", got)
		}
		if _, err := m("mongodb://localhost:27017/admin", "db.adminCommand({listDatabases:1})"); err == nil {
			t.Fatalf("an unauthenticated admin command succeeded")
		}
		t.Logf("smoke test passed (mongodb %s: auth bootstrap, document roundtrip, unauthenticated admin refused; uid 1001)", ver)
	})
}
