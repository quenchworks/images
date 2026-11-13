package smoke

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// rootLikeUpstream fails unless the image runs as root, as upstream's chart expects.
func (t *T) rootLikeUpstream() string {
	u := t.Docker("inspect", t.Image, "--format", "{{.Config.User}}")
	if u != "0" && u != "" {
		t.Fatalf("expected root like upstream, got '%s'", u)
	}
	return u
}

// csiSidecar starts a CSI sidecar against a socket nobody serves and checks it keeps
// running and tries the socket.
func (t *T) csiSidecar(args ...string) {
	dir := filepath.Dir(t.DeadKubeconfig())
	name := t.Name("")
	t.Start(name, []string{"-v", dir + ":/csi"}, args...)
	time.Sleep(6 * time.Second)
	if s := t.Docker("inspect", "-f", "{{.State.Status}}", name); s != "running" {
		t.Fatalf("sidecar exited (%s)\n%s", s, t.Logs(name))
	}
	t.ContainsFold(t.Logs(name), "csi.sock", "connection attempt to the CSI socket")
}

func init() {
	register("argo-workflows", func(t *T) {
		semver := `(?i)v?[0-9]+\.[0-9]+\.[0-9]+`
		t.Match(t.RunEntryAny("/usr/bin/argo", "version"), `(?i)argo:\s*v?[0-9]+\.[0-9]+\.[0-9]+`, "argo version")
		t.Match(t.RunAny("version"), semver, "workflow-controller version")
		t.Match(t.RunEntryAny("argoexec", "version"), semver, "argoexec version")
		help := t.RunEntryAny("argoexec", "--help")
		for _, sub := range []string{"init", "wait", "emissary"} {
			t.Match(help, `(?m)^\s+`+sub+`(\s|$)`, "argoexec subcommand "+sub)
		}
		t.Logf("smoke test passed (nonroot user: %s)", t.User("1001"))
	})
	register("busybox", func(t *T) {
		out, err := t.RunFlags([]string{"--read-only", "--entrypoint", "/bin/sh"}, "-c", `set -e
echo hello | sed s/hello/sed-ok/
echo grepline | grep grep >/dev/null && echo grep-ok
echo "a b c" | awk '{print $2}' | grep -q b && echo awk-ok
wget --help 2>&1 | head -1 >/dev/null && echo wget-ok
for a in sh ls cat cp mv rm mkdir ln sed grep awk wget; do [ -L "/bin/$a" ] || { echo "missing applet symlink: $a"; exit 1; }; done
echo applets-ok`)
		if err != nil {
			t.Fatalf("core applets: %s", out)
		}
		for _, w := range []string{"sed-ok", "grep-ok", "awk-ok", "wget-ok", "applets-ok"} {
			t.Contains(out, w, "applets")
		}
		if uid, _ := t.RunFlags([]string{"--read-only", "--entrypoint", "/bin/id"}, "-u"); uid != "1001" {
			t.Fatalf("expected uid 1001, got '%s'", uid)
		}
		t.Logf("smoke test passed (nonroot uid: %s)", t.User("1001"))
	})
	register("csi-livenessprobe", func(t *T) {
		u := t.rootLikeUpstream()
		if t.Want != "" {
			cid := t.Docker("create", t.Image)
			defer docker("rm", cid)
			bin := filepath.Join(t.TempDir(), "bin")
			t.Docker("cp", cid+":/livenessprobe", bin)
			b, _ := os.ReadFile(bin)
			t.Contains(string(b), "v"+t.Want, "binary stamped")
		}
		t.csiSidecar("--csi-address=/csi/csi.sock", "--v=5")
		t.Logf("smoke test passed (csi-livenessprobe; user %q)", u)
	})
	for _, app := range []string{"csi-attacher", "csi-provisioner", "csi-resizer", "csi-snapshotter"} {
		register(app, func(t *T) {
			u := t.rootLikeUpstream()
			if t.Want != "" {
				t.Contains(t.Run("--version"), "v"+t.Want, "version")
			}
			t.csiSidecar("--csi-address=/csi/csi.sock", "--kubeconfig=/csi/kubeconfig", "--v=5")
			t.Logf("smoke test passed (%s; user %q)", t.App, u)
		})
	}
	register("dependency-track-frontend", func(t *T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "api.conf"), []byte(`location /api/ { default_type text/plain; return 200 "conf.d include works\n"; }`+"\n"), 0o644)
		base := "http://127.0.0.1:18081"
		_, idx := t.httpUp([]string{"-p", "127.0.0.1:18081:8080", "--tmpfs", "/tmp", "--read-only", "-v", dir + ":/etc/dependency-track-frontend/conf.d:ro"}, base+"/", 30)
		t.ContainsFold(idx, "dependency-track", "index.html is the UI")
		r, err := httpc.Get(base + "/static/config.json")
		if err != nil {
			t.Fatalf("config.json: %v", err)
		}
		r.Body.Close()
		if !strings.Contains(r.Header.Get("Cache-Control"), "no-store") {
			t.Fatalf("config.json is cacheable")
		}
		_, cfg := Get(base + "/static/config.json")
		t.Contains(cfg, `"API_BASE_URL"`, "config.json")
		_, spa := Get(base + "/projects")
		t.ContainsFold(spa, "dependency-track", "SPA fallback")
		if _, b := Get(base + "/api/version"); strings.TrimSpace(b) != "conf.d include works" {
			t.Fatalf("conf.d include not applied: %s", b)
		}
		t.Logf("smoke test passed (nonroot user: %s, read-only rootfs, UI and conf.d hook)", t.User("1001"))
	})
	register("guacd", func(t *T) {
		u := t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"--read-only", "-p", "127.0.0.1:4822:4822"})
		t.WaitLog(name, "Listening on host", 30)
		if t.Want != "" {
			t.Contains(t.Logs(name), "version "+t.Want+" started", "guacd version")
		}
		for _, p := range []string{"rdp", "vnc", "ssh", "telnet"} {
			c, err := net.DialTimeout("tcp", "127.0.0.1:4822", 5*time.Second)
			if err != nil {
				t.Fatalf("dial: %v", err)
			}
			c.SetDeadline(time.Now().Add(10 * time.Second))
			fmt.Fprintf(c, "6.select,%d.%s;", len(p), p)
			r, _ := bufio.NewReader(c).ReadString(';')
			c.Close()
			t.Match(r, `^4\.args,`, p+" args reply")
			t.Contains(r, "8.hostname", p+" args")
		}
		t.Logf("smoke test passed (guacd: rdp, vnc, ssh, telnet; nonroot user: %s)", u)
	})
	register("tailscale", func(t *T) {
		for _, c := range [][]string{{"tailscale", "version"}, {"tailscaled", "--version"}} {
			v := strings.SplitN(t.RunEntryAny("/usr/local/bin/"+c[0], c[1]), "\n", 2)[0]
			t.Logf("%s: %s", c[0], v)
			if t.Want != "" && v != t.Want {
				t.Fatalf("%s version '%s', expected %s", c[0], v, t.Want)
			}
		}
		name := t.Name("")
		t.Start(name, nil)
		re := regexp.MustCompile(`"BackendState": *"NeedsLogin"`)
		for i := 0; ; i++ {
			if out, _ := docker("exec", name, "/usr/local/bin/tailscale", "--socket=/tmp/tailscale/tailscaled.sock", "status", "--json"); re.MatchString(out) {
				break
			}
			if i == 30 {
				t.Fatalf("tailscaled did not come up\n%s", t.Logs(name))
			}
			time.Sleep(time.Second)
		}
		t.Logf("smoke test passed (daemon up, NeedsLogin; user %s)", t.Docker("inspect", t.Image, "--format", "{{.Config.User}}"))
	})
	register("telegraf", func(t *T) {
		t.Match(t.RunAny("--version"), `(?i)Telegraf\s+v?[0-9]+\.[0-9]+\.[0-9]+`, "version stamped")
		cfg := filepath.Join(t.TempDir(), "telegraf.conf")
		os.WriteFile(cfg, []byte("[agent]\n  omit_hostname = true\n[[inputs.mem]]\n[[outputs.file]]\n  files = [\"stdout\"]\n"), 0o644)
		out, _ := t.RunFlags([]string{"-v", cfg + ":/etc/telegraf/telegraf.conf:ro"}, "--config", "/etc/telegraf/telegraf.conf", "--test")
		t.Match(out, `(?m)^> *mem `, "mem metric from --test")
		t.Logf("smoke test passed (nonroot user: %s)", t.User("1001"))
	})
	register("tomee", func(t *T) {
		name := t.Name("")
		t.Start(name, []string{"-p", "8080"})
		url := "http://localhost:" + t.hostPort(name, "8080/tcp") + "/"
		var c int
		for i := 0; ; i++ {
			if c, _ = Get(url); c == 200 || c == 404 {
				break
			}
			if i == 90 {
				t.Fatalf("tomee did not answer on %s (last: %d)\n%s", url, c, t.Logs(name))
			}
			time.Sleep(time.Second)
		}
		logs := t.Logs(name)
		t.Contains(logs, "org.apache.openejb", "OpenEJB startup")
		t.Contains(logs, "Server startup in", "server startup")
		if strings.Contains(logs, "SEVERE") {
			t.Fatalf("startup logged errors:\n%s", logs)
		}
		t.Logf("smoke test passed (HTTP %d, OpenEJB up, user %s)", c, t.Docker("inspect", t.Image, "--format", "{{.Config.User}}"))
	})
	register("cluster-autoscaler", func(t *T) {
		dir := filepath.Dir(t.DeadKubeconfig())
		out, _ := t.RunFlags([]string{"-v", dir + ":/kc:ro"}, "/cluster-autoscaler", "--v=1", "--cloud-provider=kwok", "--kubeconfig=/kc/kubeconfig")
		m := regexp.MustCompile(`Cluster Autoscaler ([0-9]+\.[0-9]+\.[0-9]+)`).FindStringSubmatch(out)
		if m == nil {
			t.Fatalf("no version line:\n%s", out)
		}
		if t.Want != "" && m[1] != t.Want {
			t.Fatalf("version %s, expected %s", m[1], t.Want)
		}
		t.Match(out, `Failed to get nodes from apiserver: .*127\.0\.0\.1:1`, "reached the API server call")
		if ep := t.Docker("inspect", t.Image, "--format", "{{json .Config.Entrypoint}}"); ep != "null" && ep != "[]" {
			t.Fatalf("unexpected ENTRYPOINT %s", ep)
		}
		t.Logf("smoke test passed (version %s)", m[1])
	})
}
