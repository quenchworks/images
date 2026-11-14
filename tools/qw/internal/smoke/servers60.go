package smoke

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// KubeVirt CDI control plane: each binary checks its required environment, then must reach the
// API server step against a dead kubeconfig (127.0.0.1:1) without crashing first.
func init() {
	images := []string{"IMPORTER_IMAGE", "CLONER_IMAGE", "UPLOADSERVER_IMAGE", "OVIRT_POPULATOR_IMAGE"}
	cdi := func(app string, env func(t *T) []string, args ...string) {
		register(app, func(t *T) {
			u := t.User("1001")
			t.LabelVersion()
			flags := []string{"--read-only", "--tmpfs", "/tmp", "-v", t.DeadKubeconfig() + ":/w/kubeconfig:ro", "-e", "KUBECONFIG=/w/kubeconfig"}
			for _, e := range env(t) {
				flags = append(flags, "-e", e)
			}
			out, _ := t.RunAnyFor(90*time.Second, flags, args...)
			t.Contains(out, "127.0.0.1:1", app+" reached the API server step")
			t.Logf("smoke test passed (%s %s dials the API server; nonroot user %s)", app, t.Want, u)
		})
	}
	cdi("cdi-operator", func(t *T) []string {
		e := []string{"OPERATOR_VERSION=v" + t.Want, "DEPLOY_CLUSTER_RESOURCES=true", "VERBOSITY=1", "PULL_POLICY=IfNotPresent",
			"CONTROLLER_IMAGE=x", "APISERVER_IMAGE=x", "UPLOAD_PROXY_IMAGE=x", "UPLOAD_SERVER_IMAGE=x"}
		for _, i := range images {
			e = append(e, i+"=x")
		}
		return e
	})
	cdi("cdi-controller", func(t *T) []string {
		// the operator gives the controller pod a readiness probe of `cat /tmp/ready`
		t.Contains(t.RunEntryAny("cat", "/etc/passwd"), "cdi:", "cat for the readiness probe")
		e := []string{"UPLOADPROXY_SERVICE=cdi-uploadproxy"}
		for _, i := range images {
			e = append(e, i+"=x")
		}
		return e
	}, "-kubeconfig=/w/kubeconfig") // the controller ignores $KUBECONFIG
	cdi("cdi-apiserver", func(*T) []string { return nil })
	// the upload proxy verifies upload tokens with the API server's public key, read at startup
	cdi("cdi-uploadproxy", func(t *T) []string {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("rsa key: %v", err)
		}
		der, _ := x509.MarshalPKIXPublicKey(&k.PublicKey)
		return []string{"APISERVER_PUBLIC_KEY=" + string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))}
	})

	// The importer's real job: fetch a qcow2 over HTTP (nbdkit curl plugin) and convert it to the raw
	// disk.img in /data (qemu-img), as an importer pod does for an http DataVolume.
	register("cdi-importer", func(t *T) {
		u := t.User("1001")
		t.LabelVersion()
		src, data := t.TempDir(), t.TempDir()
		t.Docker("run", "--rm", "--entrypoint", "qemu-img", "-v", src+":/w", t.Image, "create", "-f", "qcow2", "/w/src.qcow2", "16M")
		term := filepath.Join(t.TempDir(), "termination-log")
		os.WriteFile(term, nil, 0o666)
		os.Chmod(term, 0o666)
		url := t.hostStub(http.FileServer(http.Dir(src)))
		flags := append(append([]string{}, hostStubFlag...), "-v", data+":/data", "-v", term+":/dev/termination-log",
			"-e", "IMPORTER_SOURCE=http", "-e", "IMPORTER_ENDPOINT="+url+"/src.qcow2", "-e", "IMPORTER_CONTENTTYPE=kubevirt",
			"-e", "IMPORTER_IMAGE_SIZE=16Mi", "-e", "FILESYSTEM_OVERHEAD=0", "-e", "PREALLOCATION=false")
		// pass 1, as the first importer pod: nbdkit opens the URL, validation runs, then CDI asks for
		// scratch space (the controller would add a scratch PVC and start a second pod)
		out, code := t.RunAnyFor(180*time.Second, flags, "-v=3")
		t.Contains(out, "nbdkit ready", "nbdkit started on the HTTP source")
		if b, _ := os.ReadFile(term); code != 0 || !strings.Contains(string(b), `"scratchSpaceRequired":true`) {
			t.Fatalf("pass 1 exited %d, termination message %q\n%s", code, b, out)
		}
		// pass 2, with scratch: download, convert to raw, resize, done
		out, code = t.RunAnyFor(180*time.Second, append(flags, "-v", t.TempDir()+":/scratch"), "-v=3")
		if b, _ := os.ReadFile(term); code != 0 || !strings.Contains(string(b), `"scratchSpaceRequired":false`) {
			t.Fatalf("pass 2 exited %d, termination message %q\n%s", code, b, out)
		}
		info := t.Docker("run", "--rm", "--entrypoint", "qemu-img", "-v", data+":/data:ro", t.Image, "info", "/data/disk.img")
		t.Contains(info, "file format: raw", "converted disk image")
		t.Contains(info, "virtual size: 16 MiB", "disk image size")
		t.Logf("smoke test passed (cdi-importer %s: qcow2 over HTTP: nbdkit validation, then scratch transfer and qemu-img conversion to raw /data/disk.img; user %s)", t.Want, u)
	})
	register("cdi-uploadserver", func(t *T) {
		u := t.User("1001")
		t.LabelVersion()
		t.Contains(t.RunEntryAny("qemu-img", "--version"), "qemu-img version", "qemu-img")
		out, _ := t.RunAnyFor(30*time.Second, nil, "-h")
		t.Contains(out, "Usage of /usr/bin/cdi-uploadserver", "upload server starts")
		t.Logf("smoke test passed (cdi-uploadserver %s starts, qemu-img present; user %s)", t.Want, u)
	})
	// the cloner's entrypoint script needs bash, du and blockdev; a filesystem clone with nothing
	// listening must get as far as the upload
	register("cdi-cloner", func(t *T) {
		u := t.User("1001")
		t.LabelVersion()
		have := t.RunEntryAny("/usr/bin/bash", "-c", "command -v du blockdev tar")
		for _, b := range []string{"/du", "/blockdev", "/tar"} {
			t.Contains(have, b, "cloner tool")
		}
		out, _ := t.RunAnyFor(60*time.Second, []string{"-e", "VOLUME_MODE=filesystem", "-e", "MOUNT_POINT=/tmp", "-e", "PREALLOCATION=false",
			"-e", "UPLOAD_URL=https://127.0.0.1:1/upload"})
		t.Contains(out, "UPLOAD_BYTES=", "startup script measured the volume")
		t.Logf("smoke test passed (cdi-cloner %s: startup script runs; user %s)", t.Want, u)
	})
}
