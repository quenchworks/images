package smoke

import (
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// clamdSay sends one z-terminated clamd command (INSTREAM with body when data is not nil) and
// returns the reply without its terminator.
func clamdSay(addr, cmd string, data []byte) (string, error) {
	c, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return "", err
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(20 * time.Second))
	if _, err := c.Write([]byte("z" + cmd + "\x00")); err != nil {
		return "", err
	}
	if data != nil {
		n := make([]byte, 4)
		binary.BigEndian.PutUint32(n, uint32(len(data)))
		c.Write(n)
		c.Write(data)
		c.Write([]byte{0, 0, 0, 0})
	}
	b, err := io.ReadAll(c)
	return strings.TrimRight(string(b), "\x00\n"), err
}

func init() {
	// clamd needs a signature database; the test writes its own one-line .hdb (the MD5 of the
	// EICAR file) instead of downloading the real one. clamd on a read-only rootfs answers PING,
	// flags EICAR through INSTREAM, passes a clean payload, and clamscan flags the file too.
	register("clamav", func(t *T) {
		u := t.User("1001")
		for _, b := range []string{"clamd", "freshclam", "clamscan"} {
			v := t.RunEntryAny("/usr/bin/"+b, "--version")
			t.Match(v, `(?m)^ClamAV [0-9]+\.[0-9]+\.[0-9]+`, b+" version")
			if t.Want != "" {
				t.Contains(v, "ClamAV "+t.Want, b+" version")
			}
		}
		// EICAR, assembled so this file is not itself a scanner match
		eicar := `X5O!P%@AP[4\PZX54(P^)7CC)7}$EICAR` + `-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*`
		work := t.TempDir()
		db := filepath.Join(work, "db")
		os.MkdirAll(db, 0o755)
		sum := md5.Sum([]byte(eicar))
		os.WriteFile(filepath.Join(db, "quench.hdb"), []byte(fmt.Sprintf("%s:%d:Quench.Test.EICAR\n", hex.EncodeToString(sum[:]), len(eicar))), 0o644)
		os.WriteFile(filepath.Join(work, "eicar.txt"), []byte(eicar), 0o644)
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", db + ":/var/lib/clamav:ro", "-p", "127.0.0.1:13310:3310"})
		const addr = "127.0.0.1:13310"
		t.waitFor(60, time.Second, name, "clamd never answered PING", func() bool {
			if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
				t.Fatalf("clamd died during startup\n%s", t.Logs(name))
			}
			r, _ := clamdSay(addr, "PING", nil)
			return r == "PONG"
		})
		hit, _ := clamdSay(addr, "INSTREAM", []byte(eicar))
		t.Match(hit, `Quench\.Test\.EICAR.* FOUND`, "EICAR detected over INSTREAM")
		if ok, _ := clamdSay(addr, "INSTREAM", []byte("nothing to see here\n")); ok != "stream: OK" {
			t.Fatalf("clean payload: %q", ok)
		}
		if _, code := t.RunAnyFor(120*time.Second, []string{"--read-only", "--tmpfs", "/tmp", "-v", db + ":/var/lib/clamav:ro",
			"-v", work + ":/scan:ro", "--entrypoint", "/usr/bin/clamscan"}, "--no-summary", "/scan/eicar.txt"); code != 1 {
			t.Fatalf("clamscan exit %d, expected 1 (virus found)", code)
		}
		if regexp.MustCompile(`Read-only file system|[Pp]ermission denied|ERROR`).MatchString(t.Logs(name)) {
			t.Fatalf("errors in the logs\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed (clamav %s: read-only rootfs, PING, EICAR found, clean OK, clamscan exit 1; user %s)", t.Want, u)
	})

	// The optimized production start (kc.sh start --optimized, baked for postgres) on a read-only
	// rootfs with tmpfs /data, /conf and /tmp: health ready, the HTTP listener, the admin console,
	// and an admin token from the bootstrap credentials.
	register("keycloak", func(t *T) {
		const http_, mgmt = "http://127.0.0.1:18080", "http://127.0.0.1:19000"
		u := t.User("1001")
		net_, pg, name := t.Name("net"), t.Name("pg"), t.Name("")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Docker("run", "-d", "--name", pg, "--network", net_, "-e", "POSTGRES_DB=keycloak", "-e", "POSTGRES_USER=keycloak",
			"-e", "POSTGRES_PASSWORD=keycloak", "postgres:16-alpine")
		t.Cleanup(func() { docker("rm", "-f", pg) })
		t.waitFor(30, time.Second, pg, "postgres did not become ready", func() bool {
			_, err := docker("exec", pg, "pg_isready", "-U", "keycloak")
			return err == nil
		})
		t.Start(name, []string{"--network", net_, "--read-only", "--tmpfs", "/data:rw,mode=1777", "--tmpfs", "/conf:rw,mode=1777",
			"--tmpfs", "/tmp:rw,mode=1777", "-e", "KC_DB=postgres", "-e", "KC_DB_URL=jdbc:postgresql://" + pg + ":5432/keycloak",
			"-e", "KC_DB_USERNAME=keycloak", "-e", "KC_DB_PASSWORD=keycloak", "-e", "KC_BOOTSTRAP_ADMIN_USERNAME=admin",
			"-e", "KC_BOOTSTRAP_ADMIN_PASSWORD=admin", "-p", "127.0.0.1:18080:8080", "-p", "127.0.0.1:19000:9000"})
		up := regexp.MustCompile(`"status": *"UP"`)
		t.waitFor(90, 2*time.Second, name, "keycloak did not become ready", func() bool {
			if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
				t.Fatalf("keycloak exited\n%s", t.Logs(name))
			}
			_, b := Get(mgmt + "/health/ready")
			return up.MatchString(b)
		})
		// the main listener can still answer 503 for a moment after the management port reports ready
		t.waitFor(30, time.Second, name, "GET / on the HTTP listener never answered 200/302/303", func() bool {
			r, err := noRedirect.Get(http_ + "/")
			if err != nil {
				return false
			}
			r.Body.Close()
			return r.StatusCode == 200 || r.StatusCode == 302 || r.StatusCode == 303
		})
		if c, _ := Get(http_ + "/admin/master/console/"); c != 200 {
			t.Fatalf("admin console returned %d", c)
		}
		_, tok := Post(http_+"/realms/master/protocol/openid-connect/token", "application/x-www-form-urlencoded",
			"grant_type=password&client_id=admin-cli&username=admin&password=admin")
		t.Contains(tok, `"access_token":"`, "admin token from the bootstrap credentials")
		t.Logf("smoke test passed (keycloak %s: optimized start on a read-only rootfs, health UP, console, admin token; user %s)", t.Want, u)
	})
}
