package smoke

import (
	"net/http"
	"strings"
	"time"
)

func init() {
	// On a read-only rootfs as 1001 with no config: the server starts on 2801 and logs the stamped version, /ping and
	// /healthz answer, a Falco-shaped event posted to / is accepted with no outputs configured, and /metrics counts it.
	register("falcosidekick", func(t *T) {
		const base = "http://127.0.0.1:12801"
		u := t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-p", "127.0.0.1:12801:2801"})
		t.waitCode(base+"/healthz", 30, time.Second, name, 200)
		if c, b := restDo("GET", base+"/ping", "", "", ""); c != 200 || !strings.Contains(b, "pong") {
			t.Fatalf("/ping got %d %q", c, b)
		}
		ev := `{"output":"smoke","priority":"Notice","rule":"smoke","time":"2026-01-01T00:00:00Z","output_fields":{"proc.name":"smoke"}}`
		req, _ := http.NewRequest("POST", base+"/", strings.NewReader(ev))
		req.Header.Set("Content-Type", "application/json")
		if c, b := insecureDo(req); c != 200 {
			t.Fatalf("POST / got %d %q\n%s", c, b, t.Logs(name))
		}
		// the events counter is a labelled vector: it appears in /metrics only once an event has been counted
		if c, b := restDo("GET", base+"/metrics", "", "", ""); c != 200 || !strings.Contains(b, "falcosecurity_falcosidekick_falco_events_total") {
			t.Fatalf("/metrics got %d without falcosecurity_falcosidekick_falco_events_total", c)
		}
		if logs := t.Logs(name); !strings.Contains(logs, "Falcosidekick version: "+t.Want) {
			t.Fatalf("no version %s in the log:\n%s", t.Want, logs)
		}
		t.Logf("smoke test passed (falcosidekick %s: read-only boot, /ping, /healthz, /metrics, event accepted; user %s)", t.Want, u)
	})
}
