package smoke

import (
	"strings"
	"time"
)

func init() {
	// The API daemon starts on a read-only root filesystem (home and /tmp on tmpfs), reports its version, and serves
	// the network add-on's API component. That add-on is what replaces commons-httpclient's sender (the VEX for
	// CVE-2012-5783 depends on it loading). A spider run against ZAP's own API page proves the scanner works.
	register("zap", func(t *T) {
		// ZAP answers as its API only to requests addressed to its own host:port; any other Host is proxied, so the
		// port is the same inside and out.
		const base = "http://127.0.0.1:18118"
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--tmpfs", "/home/zap:rw,uid=1001,gid=1001", "-p", "127.0.0.1:18118:18118"},
			"-daemon", "-host", "0.0.0.0", "-port", "18118", "-dir", "/home/zap/.ZAP",
			"-config", "start.checkForUpdates=false", "-config", "start.checkAddonUpdates=false",
			"-config", "api.addrs.addr.name=.*", "-config", "api.addrs.addr.regex=true", "-config", "api.disablekey=true")
		var ver string
		t.waitFor(120, 2*time.Second, name, "ZAP API did not answer", func() bool {
			c, b := Get(base + "/JSON/core/view/version/")
			ver = b
			return c == 200
		})
		t.WantVersion(ver)
		c, b := Get(base + "/JSON/network/view/getConnectionTimeout/")
		if c != 200 || !strings.Contains(b, "getConnectionTimeout") {
			t.Fatalf("network add-on API not served (%d): %s\n%s", c, b, t.Logs(name))
		}
		_, sp := Get(base + "/JSON/spider/action/scan/?url=" + "http://127.0.0.1:18118/")
		t.Contains(sp, `"scan"`, "spider scan started")
		t.waitFor(120, 2*time.Second, name, "spider did not finish", func() bool {
			_, st := Get(base + "/JSON/spider/view/status/?scanId=0")
			return strings.Contains(st, `"100"`)
		})
		_, urls := Get(base + "/JSON/core/view/urls/")
		t.Contains(urls, "127.0.0.1:18118/OTHER", "spidered URLs")
		t.Logf("smoke test passed (zap %s: API daemon on a read-only rootfs, network add-on loaded, spider run; user %s)", t.Want, t.User("1001"))
	})
}
