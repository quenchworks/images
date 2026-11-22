package smoke

import "strings"

func init() {
	// The scanner reports its own version, then runs a real scan: two HTTP templates against a busybox httpd on a
	// private network, one that must match the page's marker and one that must not. Templates come from a volume
	// (nothing is fetched: -duc, -ni), as on a read-only root filesystem.
	register("nuclei", func(t *T) {
		out, _ := t.RunFlags(nil, "-version", "-duc")
		t.Logf("nuclei -version:\n%s", out)
		t.Match(out, `Nuclei Engine Version: v[0-9]`, "engine version")
		t.WantVersion(out)
		net_, tgt, vol := t.Name("net"), t.Name("target"), t.Name("tpl")
		t.Docker("network", "create", net_)
		t.Cleanup(func() { docker("network", "rm", net_) })
		t.Docker("run", "-d", "--name", tgt, "--network", net_, "--network-alias", "target", busyboxImage, "-c",
			"mkdir -p /tmp/w && echo quench-nuclei-marker > /tmp/w/index.html && exec httpd -f -p 8080 -h /tmp/w")
		t.Cleanup(func() { docker("rm", "-f", tgt) })
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		tpl := func(id, word string) string {
			return "id: " + id + "\ninfo:\n  name: " + id + "\n  author: quenchworks\n  severity: info\nhttp:\n  - method: GET\n    path: [\"{{BaseURL}}/\"]\n    matchers:\n      - type: word\n        words: [\"" + word + "\"]\n"
		}
		t.Docker("run", "--rm", "-v", vol+":/t", "--user", "0", busyboxImage, "-c",
			"printf '%s' \"$0\" > /t/hit.yaml && printf '%s' \"$1\" > /t/miss.yaml && chmod 0644 /t/*.yaml",
			tpl("quench-hit", "quench-nuclei-marker"), tpl("quench-miss", "not-on-this-page"))
		res, err := t.RunFlags([]string{"--network", net_, "--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--tmpfs", "/home/nuclei:rw,uid=1001,gid=1001", "-v", vol + ":/t:ro"},
			"-duc", "-ni", "-silent", "-jsonl", "-t", "/t/hit.yaml", "-t", "/t/miss.yaml", "-u", "http://target:8080/")
		if err != nil {
			t.Fatalf("scan failed: %v\n%s", err, res)
		}
		t.Contains(res, `"template-id":"quench-hit"`, "matching template")
		if strings.Contains(res, "quench-miss") {
			t.Fatalf("the non-matching template matched:\n%s", res)
		}
		t.Logf("smoke test passed (nuclei %s: version, read-only scan of a live target, hit and miss templates; user %s)", t.Want, t.User("1001"))
	})
}
