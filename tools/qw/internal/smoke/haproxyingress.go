package smoke

import (
	"strings"
	"time"
)

func init() {
	// No cluster here, so this checks the pieces that run standalone; the chart's kind gate runs the controller.
	// The controller reports its stamped version, HAProxy runs through haproxy_wrapper (which preloads
	// libblock_secrets.so) and accepts the shipped haproxy.cfg, and gopherd (PID 1, as 1001) parses its config and
	// renders the memory split from the cgroup limit before starting HAProxy.
	register("haproxy-kubernetes-ingress", func(t *T) {
		u := t.User("1001")
		t.Contains(t.RunEntry("/haproxy-ingress-controller", "--version"), "v"+t.Want, "controller --version")
		t.Contains(t.RunEntry("/usr/local/sbin/haproxy_wrapper", "-v"), "HAProxy version 3.4", "haproxy_wrapper -v")
		if out, err := t.RunFlags([]string{"--entrypoint", "/usr/local/sbin/haproxy_wrapper"},
			"-c", "-f", "/usr/local/etc/haproxy/haproxy.cfg"); err != nil || strings.Contains(out, "[ALERT]") {
			// exit 0 with warnings is the expected result: the base config has no bind lines until the controller
			// writes them, so HAProxy prints "Warnings were found." instead of "Configuration file is valid"
			t.Fatalf("haproxy -c on the controller base config: %v\n%s", err, out)
		}
		name := t.Name("")
		t.Start(name, []string{"--memory", "512m"})
		t.waitFor(30, time.Second, name, "gopherd never rendered the memory split", func() bool {
			return strings.Contains(t.Logs(name), "Memory limit")
		})
		t.Contains(t.Logs(name), "HAProxy: 337MiB", "memory split from the 512 MiB limit")
		t.Logf("smoke test passed (haproxy-kubernetes-ingress %s: controller version, haproxy via the wrapper, shipped config valid, gopherd memory split; user %s)", t.Want, u)
	})
}
