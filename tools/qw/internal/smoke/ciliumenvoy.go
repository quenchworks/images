package smoke

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

const ciliumEnvoyBootstrap = `admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 19000 }
static_resources:
  listeners:
    - name: smoke
      address:
        socket_address: { address: 0.0.0.0, port_value: 8080 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                stat_prefix: smoke
                route_config:
                  name: smoke
                  virtual_hosts:
                    - name: smoke
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/" }
                          direct_response:
                            status: 200
                            body: { inline_string: "quench-cilium-envoy" }
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
`

// cilium-envoy: the Envoy binary serves a static listener, reports the
// Envoy release the image is versioned by, and the starter and healthcheck binaries the cilium
// chart's DaemonSet runs are present and executable.
func init() {
	register("cilium-envoy", func(t *T) {
		u := t.User("1001")
		ev := t.RunEntry("/usr/bin/cilium-envoy", "--version")
		t.Logf("  %s", ev)
		if t.Want != "" && !strings.Contains(ev, "/"+t.Want+"/") {
			t.Fatalf("expected Envoy %s in the version banner, got %q", t.Want, ev)
		}
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "envoy.yaml"), []byte(ciliumEnvoyBootstrap), 0o644)
		name := t.Name("")
		t.Cleanup(func() { docker("rm", "-f", name) })
		// a bind mount of a temp dir is refused under Docker Desktop: docker cp before start (which a
		// --read-only container refuses too, so the rootfs stays writable here)
		t.Docker("create", "--name", name, "-p", "127.0.0.1:18083:8080",
			"--entrypoint", "/usr/bin/cilium-envoy", t.Image, "-c", "/etc/envoy.yaml", "--log-level", "warn")
		t.Docker("cp", filepath.Join(dir, "envoy.yaml"), name+":/etc/envoy.yaml")
		t.Docker("start", name)
		var body string
		t.waitFor(30, time.Second, name, "cilium-envoy never served the smoke listener", func() bool {
			c, b := Get("http://127.0.0.1:18083/")
			body = b
			return c == 200
		})
		if body != "quench-cilium-envoy" {
			t.Fatalf("unexpected body from the smoke listener: %q", body)
		}
		// no shell in the image: copy each binary out of the container to prove it is there
		for _, b := range []string{"/usr/bin/cilium-envoy-starter", "/usr/bin/cilium-envoy-healthcheck"} {
			t.Docker("cp", name+":"+b, dir)
			if fi, err := os.Stat(filepath.Join(dir, filepath.Base(b))); err != nil || fi.Mode()&0o111 == 0 {
				t.Fatalf("%s is missing or not executable", b)
			}
		}
		t.Logf("smoke test passed (cilium-envoy %s: static listener served, starter + healthcheck present; user %s)", t.Want, u)
	})
}
