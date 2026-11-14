package smoke

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func init() {
	// A serve-only controller needs an API server, but `contour bootstrap` is fully offline: it
	// builds the real Envoy bootstrap through the xDS server's go-control-plane types and writes
	// the SDS files. The test checks that output, so a canned file would fail it. Whether our
	// Envoy accepts the document belongs to the chart's kind gate, which has both images.
	register("contour", func(t *T) {
		user := t.User("1001")
		t.Contains(t.Docker("inspect", "--format", "{{json .Config.Entrypoint}}", t.Image), "/usr/bin/contour", "entrypoint")
		// `contour version` prints to stderr; RunFlags returns both streams.
		vout, _ := t.RunFlags([]string{"--read-only"}, "version")
		ver := t.Match(vout, `(?m)^version: v?(\S+)`, "version line")
		sha := t.Match(vout, `(?m)^sha: (\S+)`, "sha line")
		if ver == "0.0.0" || strings.Contains(ver, "dev") || strings.Contains(ver, "unknown") {
			t.Fatalf("version not stamped: %q", ver)
		}
		if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(sha) {
			t.Fatalf("build sha not stamped: %q", sha)
		}
		if t.Want != "" && ver != t.Want {
			t.Fatalf("expected version %s, got %s", t.Want, ver)
		}
		help, _ := t.RunFlags([]string{"--read-only"}, "--help")
		for _, c := range []string{"serve", "bootstrap", "certgen", "gateway-provisioner", "envoy shutdown-manager", "envoy shutdown", "cli cds", "version"} {
			t.Match(help, `(?m)^`+regexp.QuoteMeta(c), "subcommand "+c)
		}
		dir := t.TempDir()
		// bootstrap only records the cert paths, but stat()s them first
		for _, f := range []string{"ca.crt", "tls.crt", "tls.key"} {
			os.WriteFile(filepath.Join(dir, f), []byte("not-a-real-cert\n"), 0o644)
		}
		if out, err := t.RunFlags([]string{"--read-only", "-v", dir + ":/out"}, "bootstrap", "/out/envoy.json",
			"--xds-address=contour", "--xds-port=8001", "--xds-resource-version=v3", "--resources-dir=/out/resources",
			"--envoy-cafile=/out/ca.crt", "--envoy-cert-file=/out/tls.crt", "--envoy-key-file=/out/tls.key",
			"--admin-address=/admin/admin.sock"); err != nil {
			t.Fatalf("bootstrap failed: %s", out)
		}
		read := func(f string) []byte {
			b, err := os.ReadFile(filepath.Join(dir, f))
			if err != nil || len(b) == 0 {
				t.Fatalf("bootstrap did not write %s", f)
			}
			return b
		}
		type grpc struct {
			APIType      string `json:"api_type"`
			GrpcServices []struct {
				EnvoyGrpc struct {
					ClusterName string `json:"cluster_name"`
				} `json:"envoy_grpc"`
			} `json:"grpc_services"`
		}
		type source struct {
			grpc
			APIConfigSource *grpc `json:"api_config_source"`
		}
		var doc struct {
			DynamicResources map[string]source `json:"dynamic_resources"`
			StaticResources  struct {
				Clusters []struct {
					Name           string `json:"name"`
					LoadAssignment struct {
						Endpoints []struct {
							LbEndpoints []struct {
								Endpoint struct {
									Address struct {
										SocketAddress struct {
											Address   string `json:"address"`
											PortValue int    `json:"port_value"`
										} `json:"socket_address"`
									} `json:"address"`
								} `json:"endpoint"`
							} `json:"lb_endpoints"`
						} `json:"endpoints"`
					} `json:"load_assignment"`
				} `json:"clusters"`
			} `json:"static_resources"`
			Admin struct {
				Address struct {
					Pipe struct {
						Path string `json:"path"`
					} `json:"pipe"`
				} `json:"address"`
			} `json:"admin"`
		}
		env := read("envoy.json")
		if err := json.Unmarshal(env, &doc); err != nil {
			t.Fatalf("envoy.json is not JSON: %v", err)
		}
		// every LDS/CDS/ADS stream points at the contour xDS cluster over gRPC
		n := 0
		for _, k := range []string{"lds_config", "cds_config", "ads_config"} {
			s, ok := doc.DynamicResources[k]
			if !ok {
				continue
			}
			g := s.grpc
			if s.APIConfigSource != nil {
				g = *s.APIConfigSource
			}
			if g.APIType != "GRPC" || len(g.GrpcServices) == 0 || g.GrpcServices[0].EnvoyGrpc.ClusterName != "contour" {
				t.Fatalf("%s does not point at the contour cluster over gRPC: %s", k, env)
			}
			n++
		}
		if n == 0 {
			t.Fatalf("no xDS config sources in envoy.json")
		}
		found := false
		for _, c := range doc.StaticResources.Clusters {
			if c.Name == "contour" && len(c.LoadAssignment.Endpoints) > 0 && len(c.LoadAssignment.Endpoints[0].LbEndpoints) > 0 {
				a := c.LoadAssignment.Endpoints[0].LbEndpoints[0].Endpoint.Address.SocketAddress
				found = a.Address == "contour" && a.PortValue == 8001
			}
		}
		if !found {
			t.Fatalf("static cluster contour does not carry contour:8001")
		}
		if doc.Admin.Address.Pipe.Path != "/admin/admin.sock" {
			t.Fatalf("admin socket is %q", doc.Admin.Address.Pipe.Path)
		}
		for _, f := range []string{"resources/sds/xds-tls-certificate.json", "resources/sds/xds-validation-context.json"} {
			var v any
			if json.Unmarshal(read(f), &v) != nil {
				t.Fatalf("%s is not JSON", f)
			}
		}
		t.Logf("PASS: contour smoke test green (version %s, sha %s, nonroot uid %s)", ver, sha[:12], user)
	})
}

func init() {
	// One image, three control-plane binaries; each must start and report the stamped version.
	register("volcano", func(t *T) {
		u := t.User("1001")
		t.LabelVersion()
		for _, c := range []string{"scheduler", "controller-manager", "webhook-manager"} {
			out, err := t.RunFlags([]string{"--read-only", "--entrypoint", "/vc-" + c}, "--version")
			if err != nil {
				t.Fatalf("vc-%s --version: %s", c, out)
			}
			v := t.Match(out, `(?m)^Version: v(\S+)`, "vc-"+c+" version")
			if t.Want != "" && v != t.Want {
				t.Fatalf("vc-%s reports %s, expected %s", c, v, t.Want)
			}
		}
		t.Logf("smoke test passed (volcano %s: scheduler, controller-manager and webhook-manager start and report their version; nonroot user %s)", t.Want, u)
	})
}
