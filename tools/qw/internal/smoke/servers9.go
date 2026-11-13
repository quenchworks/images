package smoke

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// registrar checks a CSI node-driver-registrar build; want is the version it must report.
func registrar(want string) func(*T) {
	return func(t *T) {
		u := t.rootLikeUpstream()
		if w := want; w != "" || t.Want != "" {
			if w == "" {
				w = t.Want
			}
			t.Contains(t.Run("--version"), "v"+w, "version")
		}
		t.csiSidecar("--csi-address=/csi/csi.sock", "--kubelet-registration-path=/var/lib/kubelet/plugins/smoke/csi.sock", "--v=5")
		if out, err := t.RunFlags([]string{"--entrypoint", "/bin/sh"}, "-c", "rm -rf /tmp/x"); err != nil {
			t.Fatalf("no /bin/sh for the preStop hook: %s", out)
		}
		t.Logf("smoke test passed (%s; user %q)", t.App, u)
	}
}

func init() {
	register("csi-node-driver-registrar", registrar(""))
	register("calico-node-driver-registrar", registrar("2.18.0"))
	register("buildah", func(t *T) {
		const base = "ghcr.io/quenchworks/images/crane:0.22.1"
		u := t.User("1001")
		v := t.Run("--version")
		if t.Want != "" {
			t.Match(v, `^buildah version `+regexp.QuoteMeta(t.Want)+` `, "version")
		}
		ctx := t.TempDir()
		os.WriteFile(filepath.Join(ctx, "Containerfile"), []byte("FROM "+base+"\nLABEL quench.smoke=built-by-buildah\n"), 0o644)
		vol := t.Name("vol")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		if r, _ := t.RunFlags([]string{"--privileged"}, "--storage-driver", "vfs", "info", "--format", "{{.host.rootless}}"); r != "true" {
			t.Fatalf("buildah is not running rootless: %s", r)
		}
		store := vol + ":/home/nonroot/.local/share/containers"
		if out, err := t.RunFlags([]string{"--privileged", "--tmpfs", "/var/tmp:exec,mode=1777", "-v", store, "-v", ctx + ":/ctx:ro"},
			"--storage-driver", "vfs", "build", "--isolation", "chroot", "-t", "smoke:latest", "-f", "/ctx/Containerfile", "/ctx"); err != nil {
			t.Fatalf("buildah build failed:\n%s", out)
		}
		label, _ := t.RunFlags([]string{"--privileged", "-v", store}, "--storage-driver", "vfs", "inspect", "--type", "image",
			"--format", `{{index .OCIv1.Config.Labels "quench.smoke"}}`, "smoke:latest")
		if label != "built-by-buildah" {
			t.Fatalf("built image lacks the label: %s", label)
		}
		t.Logf("smoke test passed (rootless build from a Containerfile; nonroot user: %s)", u)
	})
	register("concourse-git-resource", func(t *T) {
		out, err := t.RunFlags([]string{"--entrypoint", "/bin/bash"}, "-c", `set -euo pipefail
git init -q -b main /tmp/repo
cd /tmp/repo && echo one > f && git add f && git commit -q -m one
first="$(git rev-parse HEAD)"
echo two > f && git commit -q -am two
head="$(git rev-parse HEAD)"
src="{\"uri\":\"file:///tmp/repo\",\"branch\":\"main\"}"
checked="$(echo "{\"source\":$src}" | /opt/resource/check)"
jq -e --arg h "$head" "any(.[]; .ref == \$h)" <<<"$checked" >/dev/null
got="$(echo "{\"source\":$src,\"version\":{\"ref\":\"$first\"}}" | /opt/resource/in /tmp/out)"
jq -e --arg f "$first" ".version.ref == \$f" <<<"$got" >/dev/null
[ "$(cat /tmp/out/f)" = one ]
[ "$(git -C /tmp/out rev-parse HEAD)" = "$first" ]
git lfs version >/dev/null && git-crypt --version >/dev/null && gpg --version >/dev/null && ssh -V 2>/dev/null
proxytunnel --version 2>&1 | head -1
echo resource-ok`)
		if err != nil {
			t.Fatalf("the resource protocol run failed:\n%s", out)
		}
		t.Match(out, `(?m)^resource-ok$`, "resource protocol run finished")
		t.Logf("smoke test passed (check found HEAD, in fetched a pinned ref, uid %s)", t.User("1001"))
	})
	register("flux-kustomize-controller", func(t *T) {
		u := t.User("1001")
		kc := t.DeadKubeconfig()
		out, _ := t.RunAnyFor(25*time.Second, []string{"--read-only", "--tmpfs", "/tmp", "-e", "KUBECONFIG=/etc/kubeconfig", "-v", kc + ":/etc/kubeconfig:ro"})
		t.Contains(out, "127.0.0.1:1", "dialed the configured API server")
		if m := regexp.MustCompile(`panic:|read-only file system`).FindString(out); m != "" {
			t.Fatalf("crashed or wrote to the root (%s):\n%s", m, out)
		}
		for _, b := range []string{"git", "ssh", "gpg"} {
			if _, err := t.RunFlags([]string{"--entrypoint", "/usr/bin/" + b}, "--version"); err != nil {
				if _, err := t.RunFlags([]string{"--entrypoint", "/usr/bin/" + b}, "-V"); err != nil {
					t.Fatalf("%s missing or broken", b)
				}
			}
		}
		t.Logf("smoke test passed (uid %s, dead-API boot on a read-only root)", u)
	})
	register("gotify", func(t *T) {
		_, h := t.httpUp([]string{"--tmpfs", "/app/data:uid=1001,gid=1001", "-p", "127.0.0.1:8080:8080"}, "http://127.0.0.1:8080/health", 30)
		t.ContainsFold(h, "green", "health")
		_, v := Get("http://127.0.0.1:8080/version")
		t.Match(v, `"version":"[0-9]+\.[0-9]+\.[0-9]+"`, "version stamped")
		_, ui := Get("http://127.0.0.1:8080/")
		t.ContainsFold(ui, "<!doctype html", "embedded UI")
		t.Logf("smoke test passed (nonroot user: %s)", t.User("1001"))
	})
	register("gravitee-console", func(t *T) {
		u := t.User("1001")
		b := "http://127.0.0.1:18380"
		_, body := t.httpUp([]string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-p", "127.0.0.1:18380:8080"}, b+"/", 30)
		t.ContainsFold(body, "gravitee", "console index")
		js := regexp.MustCompile(`main\.[0-9a-f]+\.js`).FindString(body)
		if js == "" {
			t.Fatalf("no main.<hash>.js in index.html")
		}
		r, err := httpc.Get(b + "/" + js)
		if err != nil {
			t.Fatalf("%s: %v", js, err)
		}
		r.Body.Close()
		if ct := r.Header.Get("Content-Type"); !strings.Contains(ct, "javascript") {
			t.Fatalf("%s served as '%s'", js, ct)
		}
		_, consts := Get(b + "/constants.json")
		t.Match(consts, `"baseURL" *: *"/management"`, "constants.json")
		if _, deep := Get(b + "/some/deep/console/route"); deep != body {
			t.Fatalf("SPA deep link did not fall back to index.html")
		}
		t.Logf("smoke test passed (uid %s, console index, %s, constants.json, SPA fallback)", u, js)
	})
	register("hubble-ui-backend", func(t *T) {
		u := t.User("1001")
		home := t.TempDir()
		os.MkdirAll(filepath.Join(home, ".kube"), 0o755)
		b, _ := os.ReadFile(t.DeadKubeconfig())
		os.WriteFile(filepath.Join(home, ".kube/config"), b, 0o644)
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp", "-p", "127.0.0.1:18090:8090", "-v", home + ":/kc:ro", "-e", "HOME=/kc", "-e", "FLOWS_API_ADDR=127.0.0.1:1"})
		t.WaitTCP(30, name, "127.0.0.1:18090")
		time.Sleep(3 * time.Second)
		if s := t.Docker("inspect", name, "--format", "{{.State.Running}}"); s != "true" {
			t.Fatalf("backend exited\n%s", t.Logs(name))
		}
		logs := t.Logs(name)
		t.Contains(logs, "TLS to hubble-relay", "relay settings read")
		if regexp.MustCompile(`panic:|level=fatal`).MatchString(logs) {
			t.Fatalf("backend crashed\n%s", logs)
		}
		t.Logf("smoke test passed (uid %s, events port open, relay retry)", u)
	})
	register("ko", func(t *T) {
		u := t.User("1001")
		v := t.Run("version")
		if t.Want != "" && v != t.Want {
			t.Fatalf("ko %s, expected %s", v, t.Want)
		}
		src := t.TempDir()
		os.WriteFile(filepath.Join(src, "go.mod"), []byte("module example.com/smoke\n\ngo 1.22\n"), 0o666)
		os.WriteFile(filepath.Join(src, "main.go"), []byte("package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"built by ko\") }\n"), 0o666)
		name := t.Name("")
		t.Docker("create", "--name", name, "-w", "/tmp/app", "-e", "KO_DOCKER_REPO=example.com/smoke", t.Image,
			"build", "--push=false", "--bare", "--tarball=/tmp/out.tar", "--platform=linux/"+runtime.GOARCH, ".")
		t.Cleanup(func() { docker("rm", "-f", name) })
		t.Docker("cp", src+"/.", name+":/tmp/app")
		if out, err := docker("start", "-a", name); err != nil {
			t.Fatalf("ko build:\n%s", out)
		}
		tar := filepath.Join(t.TempDir(), "out.tar")
		t.Docker("cp", name+":/tmp/out.tar", tar)
		m := regexp.MustCompile(`Loaded image[^:]*: (\S+)`).FindAllStringSubmatch(t.Docker("load", "-i", tar), -1)
		if m == nil {
			t.Fatalf("docker load printed no image")
		}
		ref := m[len(m)-1][1]
		t.Cleanup(func() { docker("rmi", "-f", ref) })
		if got := t.Docker("run", "--rm", ref); got != "built by ko" {
			t.Fatalf("built image printed '%s'", got)
		}
		t.Logf("smoke test passed (uid %s, ko build ran the image's go toolchain)", u)
	})
	register("n8n-runners", func(t *T) {
		t.ContainsFold(t.RunAny(), "N8N_RUNNERS_AUTH_TOKEN", "launcher reached config load")
		t.RunEntry("/usr/bin/node", "--version")
		t.RunEntry("/opt/runners/task-runner-python/.venv/bin/python", "-c", "import urllib3, websockets; print('py-runner deps ok:', urllib3.__version__)")
		t.RunEntry("/usr/bin/node", "-e", "require('fs').accessSync('/opt/runners/task-runner-javascript/dist/start.js')")
		for _, pm := range []string{"uv", "poetry", "npm", "yarn", "pnpm"} {
			v, err := t.RunFlags([]string{"--entrypoint", "/usr/bin/" + pm}, "--version")
			if err != nil || v == "" {
				if v, err = t.RunFlags([]string{"--entrypoint", pm}, "--version"); err != nil || v == "" {
					t.Fatalf("package manager missing: %s", pm)
				}
			}
			t.Logf("  %s: %s", pm, strings.SplitN(v, "\n", 2)[0])
		}
		t.Logf("smoke test passed (nonroot user: %s)", t.User("1001"))
	})
}
