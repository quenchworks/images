package smoke

import (
	"os"
	"path/filepath"
	"time"
)

// k8ssandraDial runs a k8ssandra controller against an API server that never answers. Both
// find their own namespace in the service account mount (cass-operator's GetOperatorNamespace)
// and must get as far as dialing the API server for the image ConfigMap. stamp is the log text
// carrying the version, or "" for a binary upstream does not stamp.
func k8ssandraDial(stamp string) func(*T) {
	return func(t *T) {
		u := t.User("1001")
		t.LabelVersion()
		t.Contains(t.RunAny("--help"), "-health-probe-bind-address", "--help")
		kc := t.DeadKubeconfig()
		sa := t.TempDir()
		os.WriteFile(filepath.Join(sa, "namespace"), []byte("k8ssandra-operator"), 0o644)
		out, _ := t.RunAnyFor(90*time.Second, []string{"--read-only", "--tmpfs", "/tmp", "-v", kc + ":/w/kubeconfig:ro",
			"-v", sa + ":/var/run/secrets/kubernetes.io/serviceaccount:ro", "-e", "KUBECONFIG=/w/kubeconfig", "-e", "WATCH_NAMESPACE=k8ssandra-operator"})
		t.Contains(out, "watch namespace configured", "WATCH_NAMESPACE read")
		if stamp != "" && t.Want != "" {
			t.Contains(out, stamp+t.Want+" ", "stamped version")
		}
		t.Contains(out, "127.0.0.1:1", "reached the API server step")
		t.Logf("smoke test passed (%s %s: CLI, API server dial, nonroot user %s)", t.App, t.Want, u)
	}
}

func init() {
	register("k8ssandra-operator", k8ssandraDial("#### version v"))
	register("cass-operator", k8ssandraDial(""))
}
