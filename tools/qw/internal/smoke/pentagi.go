package smoke

import (
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"
)

// pentagiDind is the Docker API PentAGI's agents drive; the server will not start without one. Test-only sidecar.
const pentagiDind = "docker:27.5.1-dind"

func init() {
	// On a read-only rootfs with DATA_DIR on a volume, against the catalog's PostgreSQL (pgvector) and a Docker-in-Docker
	// sidecar: the migrations run, the seeded admin signs in and is told to change its password, a wrong password is
	// refused, the admin sets a new password, and after a restart the new password works and the seeded one does not.
	register("pentagi", func(t *T) {
		const base = "http://127.0.0.1:18117"
		const dbpw, seeded, newpw = "Smoke-Db-2026", "admin", "Smoke-Pentagi-2026!"
		u := t.User("1001")
		net := t.Name("net")
		t.Docker("network", "create", net)
		t.Cleanup(func() { docker("network", "rm", net) })
		dind := t.Name("dind")
		t.Docker("run", "-d", "--privileged", "--name", dind, "--network", net, "--network-alias", "dind", "-e", "DOCKER_TLS_CERTDIR=", pentagiDind)
		t.Cleanup(func() { docker("rm", "-f", dind) })
		db := t.Name("db")
		t.Docker("run", "-d", "--name", db, "--network", net, "--network-alias", "db", "-e", "POSTGRES_PASSWORD="+dbpw,
			"-e", "POSTGRES_USER=pentagi", "-e", "POSTGRES_DB=pentagi_db", leantimePostgres)
		t.Cleanup(func() { docker("rm", "-f", db) })
		t.waitFor(60, 2*time.Second, db, "postgres did not become ready", func() bool {
			_, err := docker("exec", "-e", "PGPASSWORD="+dbpw, db, "psql", "-h", "127.0.0.1", "-U", "pentagi", "-d", "pentagi_db", "-tAc", "select 1")
			return err == nil
		})
		t.waitFor(60, 2*time.Second, dind, "docker-in-docker did not start", func() bool {
			_, err := docker("exec", dind, "docker", "info")
			return err == nil
		})
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		flags := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--network", net, "-v", vol + ":/var/lib/pentagi", "-p", "127.0.0.1:18117:8443",
			"-e", "DATABASE_URL=postgres://pentagi:" + dbpw + "@db:5432/pentagi_db?sslmode=disable",
			"-e", "DOCKER_HOST=tcp://dind:2375", "-e", "COOKIE_SIGNING_SALT=quench-pentagi-smoke-salt-0123456789"}
		start := func(name string) {
			t.Start(name, flags)
			t.waitFor(90, 2*time.Second, name, "pentagi did not answer /api/v1/info", func() bool {
				c, _ := restDo("GET", base+"/api/v1/info", "", "", "")
				return c == http.StatusOK || c == http.StatusUnauthorized
			})
		}
		client := func() *http.Client { j, _ := cookiejar.New(nil); return &http.Client{Jar: j, Timeout: 30 * time.Second} }
		do := func(c *http.Client, method, path, body string) (int, string) {
			req, _ := http.NewRequest(method, base+path, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r, err := c.Do(req)
			if err != nil {
				return 0, err.Error()
			}
			defer r.Body.Close()
			b := new(strings.Builder)
			buf := make([]byte, 32<<10)
			for {
				n, err := r.Body.Read(buf)
				b.Write(buf[:n])
				if err != nil {
					break
				}
			}
			return r.StatusCode, b.String()
		}
		login := func(c *http.Client, pw string) (int, string) {
			return do(c, "POST", "/api/v1/auth/login", `{"mail":"admin@pentagi.com","password":"`+pw+`"}`)
		}
		name := t.Name("")
		start(name)
		if c, _ := login(client(), "wrong-password-1"); c == http.StatusOK {
			t.Fatalf("a wrong password was accepted")
		}
		admin := client()
		if c, b := login(admin, seeded); c != http.StatusOK {
			t.Fatalf("seeded admin sign-in: %d %s\n%s", c, b, t.Logs(name))
		}
		_, info := do(admin, "GET", "/api/v1/info", "")
		t.Contains(info, `"password_change_required":true`, "the seeded admin must change its password")
		if c, b := do(admin, "PUT", "/api/v1/user/password", `{"current_password":"`+seeded+`","password":"`+newpw+`","confirm_password":"`+newpw+`"}`); c != http.StatusOK {
			t.Fatalf("change password: %d %s", c, b)
		}
		docker("rm", "-f", name)
		start(t.Name("again"))
		if c, _ := login(client(), seeded); c == http.StatusOK {
			t.Fatalf("the seeded password still works after the change")
		}
		if c, b := login(client(), newpw); c != http.StatusOK {
			t.Fatalf("new password after a restart: %d %s", c, b)
		}
		t.Logf("smoke test passed (pentagi %s: read-only boot against PostgreSQL/pgvector and a Docker API, migrations, seeded admin signs in and must change its password, wrong password refused, new password kept across a restart; user %s)", t.Want, u)
	})
}
