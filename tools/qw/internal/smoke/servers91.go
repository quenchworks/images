package smoke

import (
	"bufio"
	"context"
	"net/http"
	"strings"
	"time"
)

func init() {
	// On a read-only rootfs with the Docker socket: a logger container runs beside it, Dozzle
	// answers its healthcheck, and its event stream lists the logger, so the real Docker client
	// talks to the daemon as uid 1001 through the socket's group.
	register("dozzle", func(t *T) {
		const base = "http://127.0.0.1:18089"
		const sock = "/var/run/docker.sock"
		u := t.User("1001")
		t.Contains(t.RunAny("--version"), t.Want, "dozzle --version")
		bb := envOr("BUSYBOX_IMAGE", "ghcr.io/quenchworks/images/busybox@sha256:041302c8da3a2e182ecc2eac3c56c4f47a13c03cc284d255489c0be741893b64")
		t.Docker("pull", "-q", bb) // so the output below is only stat's
		out := strings.Fields(t.Docker("run", "--rm", "--user", "0", "--entrypoint", "stat", "-v", sock+":"+sock, bb, "-c", "%g", sock))
		gid := out[len(out)-1]
		logger := t.Name("logger")
		t.Docker("run", "-d", "--name", logger, "--entrypoint", "sh", bb, "-c", "while true; do echo quench-dozzle-marker; sleep 1; done")
		t.Cleanup(func() { docker("rm", "-f", logger) })
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--tmpfs", "/data:rw,uid=1001,gid=1001",
			"-v", sock + ":" + sock + ":ro", "--group-add", gid, "-p", "127.0.0.1:18089:8080"})
		t.waitFor(60, time.Second, name, "dozzle never answered /healthcheck", func() bool {
			if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
				t.Fatalf("dozzle exited\n%s", t.Logs(name))
			}
			c, _ := Get(base + "/healthcheck")
			return c == 200
		})
		// the event stream sends the container list first; read it for a few seconds
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, "GET", base+"/api/events/stream", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("event stream: %v", err)
		}
		defer resp.Body.Close()
		found := false
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 1<<20), 1<<22)
		for sc.Scan() {
			if strings.Contains(sc.Text(), logger) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("the event stream never listed the logger container %s\n%s", logger, t.Logs(name))
		}
		t.Logf("smoke test passed (dozzle %s: read-only boot, Docker socket as uid 1001 via group %s, logger container listed; user %s)", t.Want, gid, u)
	})
}
