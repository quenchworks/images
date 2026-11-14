package smoke

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
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
	cdi("cdi-controller", func(*T) []string {
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
}
