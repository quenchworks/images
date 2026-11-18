package smoke

import (
	"strings"
	"time"
)

func init() {
	// A one-member Patroni cluster against our etcd image, as the Zalando operator runs Spilo minus Kubernetes: Patroni
	// elects a primary, the preload list Spilo writes loads (bg_mon, pgextwlist, pg_auth_mon, set_user, pg_cron,
	// pg_stat_kcache), pgvector works, and the data and the primary role survive a container restart.
	register("spilo", func(t *T) {
		const api = "http://127.0.0.1:18008"
		u := t.User("101")
		net_, etcd, name, vol := t.Name("net"), t.Name("etcd"), t.Name(""), t.Name("data")
		pw := t.Name("su")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		t.Cleanup(func() { docker("rm", "-f", etcd) })
		t.Docker("run", "-d", "--name", etcd, "--network", net_, "-e", "ETCD_LISTEN_CLIENT_URLS=http://0.0.0.0:2379",
			"-e", "ETCD_ADVERTISE_CLIENT_URLS=http://"+etcd+":2379", "ghcr.io/quenchworks/images/etcd:3.7.3")
		start := func() {
			t.Start(name, []string{"--network", net_, "-p", "127.0.0.1:18008:8008", "-v", vol + ":/home/postgres/pgdata",
				"-e", "SCOPE=smoke", "-e", "ETCD3_HOSTS='" + etcd + ":2379'", "-e", "PGPASSWORD_SUPERUSER=" + pw,
				"-e", "PGPASSWORD_STANDBY=" + pw + "-standby", "-e", "ALLOW_NOSSL=true"})
		}
		primary := func() {
			t.waitFor(90, 2*time.Second, name, "Patroni never reported a primary", func() bool {
				code, _ := Get(api + "/primary")
				return code == 200
			})
		}
		psql := func(sql string) string {
			out, err := docker("exec", "-e", "PGPASSWORD="+pw, name, "psql", "-h", "127.0.0.1", "-U", "postgres", "-tAc", sql)
			if err != nil {
				t.Fatalf("psql %q: %v\n%s\n%s", sql, err, out, t.Logs(name))
			}
			return strings.TrimSpace(out)
		}
		start()
		primary()
		pre := psql("show shared_preload_libraries")
		for _, lib := range []string{"bg_mon", "pgextwlist", "pg_auth_mon", "set_user", "pg_cron", "pg_stat_kcache"} {
			t.Contains(pre, lib, "shared_preload_libraries")
		}
		t.WantVersion(psql("show server_version"))
		psql("create extension if not exists vector")
		psql("create table qw (id int primary key, v vector(3)); insert into qw values (1, '[1,2,3]')")
		t.Contains(psql("select v <-> '[1,2,4]' from qw where id = 1"), "1", "pgvector distance")
		docker("restart", name)
		primary()
		t.Contains(psql("select v from qw where id = 1"), "[1,2,3]", "row after restart")
		t.Logf("smoke test passed (uid %s, Patroni primary on etcd, Spilo preloads, pgvector, data kept across a restart)", u)
	})
}

func init() {
	// The operator against a dead API server (OPERATOR_NAMESPACE stands in for the pod's service-account namespace file):
	// it prints its stamped version, builds its client from the kubeconfig and calls the API, and
	// neither panics nor writes to the read-only root.
	register("postgres-operator", func(t *T) {
		u := t.User("1001")
		kc := t.DeadKubeconfig()
		out, _ := t.RunAnyFor(25*time.Second, []string{"--read-only", "--tmpfs", "/tmp", "-e", "OPERATOR_NAMESPACE=default", "-v", kc + ":/etc/kubeconfig:ro"},
			"-outofcluster", "-kubeconfig", "/etc/kubeconfig")
		t.Contains(out, "Spilo operator "+t.Want, "stamped version")
		// the operator logs no address: this fatal follows KubeClient.Namespaces().Get (pkg/controller/controller.go:503-504),
		// so reaching it means the client was built from the kubeconfig and called the dead API server
		t.Contains(out, `Could not find the watched namespace \"default\"`, "called the configured API server")
		if strings.Contains(out, "panic:") || strings.Contains(out, "read-only file system") {
			t.Fatalf("crashed or wrote to the root:\n%s", out)
		}
		t.Logf("smoke test passed (uid %s, version stamped, called the configured API server)", u)
	})
}
