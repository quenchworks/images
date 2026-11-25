package smoke

import (
	"net/http"
	"strings"
	"time"
)

func init() {
	// Read-only rootfs as 1001 with tmpfs over the two state dirs: docker_start.sh seeds /etc/crowdsec from /staging,
	// registers the local agent, and LAPI answers /health on 8080. cscli then sees the LAPI, a bouncer key set by env
	// is accepted by /v1/decisions, and the agent metrics come up on 6060.
	register("crowdsec", func(t *T) {
		const base = "http://127.0.0.1:18080"
		u := t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777",
			"--tmpfs", "/etc/crowdsec:rw,uid=1001,gid=1001", "--tmpfs", "/var/lib/crowdsec/data:rw,uid=1001,gid=1001",
			"-e", "NO_HUB_UPGRADE=true", "-e", "DISABLE_ONLINE_API=true", "-e", "BOUNCER_KEY_smoke=smoke-bouncer-key-0123456789",
			"-p", "127.0.0.1:18080:8080", "-p", "127.0.0.1:16060:6060"})
		t.waitCode(base+"/health", 60, time.Second, name, 200)
		if out := t.Docker("exec", name, "cscli", "lapi", "status"); !strings.Contains(out, "successfully interact") {
			t.Fatalf("cscli lapi status:\n%s", out)
		}
		req, _ := http.NewRequest("GET", base+"/v1/decisions", nil)
		req.Header.Set("X-Api-Key", "smoke-bouncer-key-0123456789")
		if c, b := insecureDo(req); c != 200 {
			t.Fatalf("/v1/decisions with the bouncer key got %d %q", c, b)
		}
		t.waitFor(30, time.Second, name, "no cs_ metrics on 6060", func() bool {
			c, b := restDo("GET", "http://127.0.0.1:16060/metrics", "", "", "")
			return c == 200 && strings.Contains(b, "cs_")
		})
		t.Contains(t.Docker("exec", name, "cscli", "version"), "v"+t.Want, "cscli version")
		t.Logf("smoke test passed (crowdsec %s: read-only boot, LAPI /health, cscli lapi status, bouncer key, metrics; user %s)", t.Want, u)
	})
}
