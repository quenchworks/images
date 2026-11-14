package smoke

import (
	"regexp"
	"strings"
	"time"
)

// adcSocketProbe runs in the container's node: the data socket is 0660 and owned by 1001, PUT
// /sync and /validate answer ADC's own 400 validation, and /healthz/ready lives on the status
// port only.
const adcSocketProbe = `const fs = require('node:fs'), http = require('node:http');
const SOCK = '/sockets/adc.sock';
const st = fs.statSync(SOCK);
const mode = (st.mode & 0o777).toString(8);
if (!st.isSocket()) { console.error('FAIL: ' + SOCK + ' is not a socket'); process.exit(1); }
if (mode !== '660') { console.error('FAIL: socket mode is 0' + mode + ', expected 0660'); process.exit(1); }
if (st.uid !== 1001) { console.error('FAIL: socket uid is ' + st.uid + ', expected 1001'); process.exit(1); }
const req = (path, method, body) => new Promise((res) => {
  const q = http.request({ socketPath: SOCK, path, method, headers: { 'content-type': 'application/json' } },
    (r) => { let d = ''; r.on('data', (c) => d += c); r.on('end', () => res([r.statusCode, d])); });
  q.on('error', (e) => res(['ERR', e.message]));
  if (body) q.write(body);
  q.end();
});
(async () => {
  const [sc, sb] = await req('/sync', 'PUT', '{}');
  if (sc !== 400 || !sb.includes('task')) { console.error('FAIL: PUT /sync gave ' + sc + ' ' + sb); process.exit(1); }
  const [vc] = await req('/validate', 'PUT', '{}');
  if (vc !== 400) { console.error('FAIL: PUT /validate gave ' + vc); process.exit(1); }
  const [hc] = await req('/healthz/ready', 'GET');
  if (hc !== 404) { console.error('FAIL: data socket served /healthz/ready with ' + hc); process.exit(1); }
  console.log('SOCKET ok');
})();`

func init() {
	// The server subcommand exists only with ADC_RUNNING_MODE=ingress; booted that way it serves
	// /healthz/ready on the status port and the sync API on a unix socket (probed from inside).
	register("adc", func(t *T) {
		const sock = "/sockets/adc.sock"
		u := t.User("1001")
		lines := strings.Split(strings.TrimSpace(t.RunAny("-v")), "\n")
		ver := strings.TrimSpace(lines[len(lines)-1])
		if ver == "" || ver == "0.0.0" || strings.Contains(ver, "dev") || strings.Contains(ver, "unknown") || (t.Want != "" && ver != t.Want) {
			t.Fatalf("adc -v %q, expected %s", ver, t.Want)
		}
		out, err := t.TryRun("server", "--help")
		if err == nil {
			t.Fatalf("'server' worked WITHOUT ADC_RUNNING_MODE=ingress; the gate moved upstream\n%s", out)
		}
		t.Contains(out, "unknown command 'server'", "failure without ADC_RUNNING_MODE")
		gout, _ := t.RunFlags([]string{"-e", "ADC_RUNNING_MODE=ingress"}, "server", "--help")
		t.Contains(gout, "--listen-status", "server --help in ingress mode")

		name := t.Name("")
		t.Start(name, []string{"-e", "ADC_RUNNING_MODE=ingress", "-e", "ADC_EXPERIMENTAL_FEATURE_FLAGS=remote-state-file,parallel-backend-request",
			"-e", "ADC_INGRESS_LOG_LEVEL=info", "-p", "127.0.0.1::3001"}, "server", "--listen", "unix:"+sock, "--listen-status", "3001")
		st := "http://127.0.0.1:" + t.hostPort(name, "3001/tcp")
		t.waitFor(40, time.Second, name, "adc never served /healthz/ready on :3001", func() bool {
			if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
				t.Fatalf("adc exited\n%s", t.Logs(name))
			}
			c, b := Get(st + "/healthz/ready")
			return c == 200 && b == "OK"
		})
		t.Contains(t.Logs(name), "ADC server is running on: "+sock, "socket startup line")
		if c, _ := DoH("PUT", st+"/sync", "{}", map[string]string{"Content-Type": "application/json"}); c != 404 {
			t.Fatalf("status port answered PUT /sync with %d, expected 404", c)
		}
		if out, err := docker("exec", name, "/usr/bin/node", "-e", adcSocketProbe); err != nil {
			t.Fatalf("socket probe failed\n%s\n%s", out, t.Logs(name))
		}
		t.Logf("smoke test passed (adc %s: ingress-mode gate, /healthz/ready, socket 0660 with the sync API; user %s)", ver, u)
	})

	// Every component symlink dispatches its own command, the React console is embedded, the
	// tools the repo server execs are present, and the repo server reports healthy on a
	// read-only rootfs.
	register("argocd", func(t *T) {
		u := t.User("1001")
		ver := t.Match(t.RunEntryAny("/usr/bin/argocd", "version", "--client", "--short"), `(?m)^argocd: (v\d+\.\d+\.\d+)$`, "stamped version")
		if t.Want != "" && ver != "v"+strings.TrimPrefix(t.Want, "v") {
			t.Fatalf("version %s, expected %s", ver, t.Want)
		}
		for bin, want := range map[string]string{
			"argocd-server":                    "gRPC/REST server which exposes the API",
			"argocd-repo-server":               "maintains a local cache of the Git repository",
			"argocd-application-controller":    "continuously monitors running applications",
			"argocd-applicationset-controller": "applicationset",
			"argocd-notifications":             "notification",
			"argocd-cmp-server":                "runs as sidecar container in reposerver",
			"argocd-commit-server":             "commits and pushes hydrated manifests",
			"argocd-dex":                       "dex",
			"argocd-k8s-auth":                  "argocd-k8s-auth",
			"argocd-git-ask-pass":              "git credential helper",
		} {
			t.ContainsFold(t.RunEntryAny("/usr/bin/"+bin, "--help"), want, bin+" dispatches its own command")
		}
		ak, _ := t.RunFlags([]string{"-e", "ARGOCD_BINARY_NAME=argocd-server", "--entrypoint", "/usr/bin/argocd"}, "--help")
		t.Contains(ak, "gRPC/REST server which exposes the API", "ARGOCD_BINARY_NAME override")

		names, err := t.RunFlags([]string{"--entrypoint", "/usr/bin/strings"}, "-n", "8", "/usr/bin/argocd")
		if err != nil {
			names = t.RunEntryAny("/bin/sh", "-c", "tr -c '[:print:]' '\\n' < /usr/bin/argocd")
		}
		t.Contains(names, "dist/app/index.html", "embedded UI index.html")
		t.Contains(names, "dist/app/assets/images/resources/", "embedded UI resource icons")
		if n := len(regexp.MustCompile(`dist/app/[0-9a-z.]*chunk\.js`).FindAllString(names, -1)); n < 10 {
			t.Fatalf("only %d embedded js chunks; expected >=10", n)
		}

		helmv := t.Match(t.RunEntryAny("/usr/bin/helm", "version", "--short"), `(?m)^(v3\.\S+)`, "helm 3.x")
		t.Match(t.RunEntryAny("/usr/bin/kustomize", "version"), `v?5\.`, "kustomize 5.x")
		t.Match(t.RunEntryAny("/usr/bin/git", "--version"), `(?m)^git version 2\.`, "git 2.x")
		for _, c := range [][]string{{"/usr/bin/git", "lfs", "version"}, {"/usr/bin/gpg", "--version"}, {"/usr/bin/gpg-wrapper.sh", "--version"}} {
			t.RunEntry(c[0], c[1:]...)
		}
		t.Contains(t.RunEntryAny("/usr/bin/git-verify-wrapper.sh"), "Wrong usage", "git-verify-wrapper.sh runs")

		name := t.Name("repo")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--tmpfs", "/helm-working-dir:rw,mode=1777", "--tmpfs", "/app/config/gpg/keys:rw,mode=1777",
			"--tmpfs", "/home/argocd:rw,mode=1777", "-p", "127.0.0.1::8084", "--entrypoint", "/usr/bin/argocd-repo-server"}, "--port", "8081", "--metrics-port", "8084")
		m := "http://127.0.0.1:" + t.hostPort(name, "8084/tcp")
		t.waitFor(30, time.Second, name, "repo-server never answered /healthz on 8084", func() bool { c, _ := Get(m + "/healthz"); return c == 200 })
		t.Docker("exec", name, "/usr/bin/helm", "version", "--short")
		if id := t.RunEntry("/usr/bin/id", "-u"); id != "1001" {
			t.Fatalf("runtime uid %q, expected 1001", id)
		}
		t.Logf("smoke test passed (argocd %s: component dispatch, UI embedded, helm %s, repo-server healthy read-only; user %s)", ver, helmv, u)
	})
}
