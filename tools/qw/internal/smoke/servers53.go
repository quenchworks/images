package smoke

import (
	"os"
	"path/filepath"
	"time"
)

func init() {
	register("k8ssandra-operator", func(t *T) {
		u := t.User("1001")
		t.LabelVersion()
		t.Contains(t.RunAny("--help"), "-health-probe-bind-address", "--help")
		// Against an API server that never answers: it must log the stamped version, then
		// dial the API server for its image ConfigMap (and fail there, not earlier). It finds its
		// own namespace in the service account mount, as cass-operator's GetOperatorNamespace does.
		kc := t.DeadKubeconfig()
		sa := t.TempDir()
		os.WriteFile(filepath.Join(sa, "namespace"), []byte("k8ssandra-operator"), 0o644)
		out, _ := t.RunAnyFor(90*time.Second, []string{"--read-only", "--tmpfs", "/tmp", "-v", kc + ":/w/kubeconfig:ro",
			"-v", sa + ":/var/run/secrets/kubernetes.io/serviceaccount:ro", "-e", "KUBECONFIG=/w/kubeconfig", "-e", "WATCH_NAMESPACE=k8ssandra-operator"})
		t.Contains(out, "watch namespace configured", "WATCH_NAMESPACE read")
		if t.Want != "" {
			t.Contains(out, "#### version v"+t.Want+" ", "stamped version")
		}
		t.Contains(out, "127.0.0.1:1", "reached the API server step")
		t.Logf("smoke test passed (k8ssandra-operator %s: CLI, stamped version, API server dial, nonroot user %s)", t.Want, u)
	})
}
